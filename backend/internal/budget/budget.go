// Package budget 月度预算控制：MySQL 用量流水为权威账本，Redis Lua 原子管理并发成本预留。
package budget

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"aegis-gateway/internal/model"
)

// ErrParse 计数解析失败。
var ErrParse = errors.New("budget counter parse error")

var reserveMonthlyBudget = redis.NewScript(`
local booking = KEYS[1]
local used_key = KEYS[2]
local reserved_key = KEYS[3]
if redis.call('EXISTS', booking) == 1 then
  return 1
end
local base = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local amount = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local month_ttl = tonumber(ARGV[5])
local used_raw = redis.call('GET', used_key)
local used = base
if used_raw then
  used = tonumber(used_raw)
  if used < base then
    used = base
    redis.call('SET', used_key, used, 'PX', month_ttl)
  end
else
  redis.call('SET', used_key, used, 'PX', month_ttl)
end
local reserved = tonumber(redis.call('GET', reserved_key) or '0')
if used + reserved + amount > limit then
  return 0
end
redis.call('INCRBY', reserved_key, amount)
redis.call('SET', booking, amount, 'PX', ttl)
redis.call('PEXPIRE', reserved_key, ttl * 2)
return 1
`)

var releaseMonthlyBudget = redis.NewScript(`
local amount = redis.call('GET', KEYS[1])
if not amount then
  return 0
end
local remaining = redis.call('DECRBY', KEYS[2], tonumber(amount))
if remaining <= 0 then
  redis.call('DEL', KEYS[2])
end
redis.call('DEL', KEYS[1])
return 1
`)

var settleMonthlyBudget = redis.NewScript(`
local used_key = KEYS[1]
local reservation_key = KEYS[2]
local reserved_key = KEYS[3]
local actual = tonumber(ARGV[1])
local month_ttl = tonumber(ARGV[2])
if actual > 0 then
  redis.call('INCRBY', used_key, actual)
  redis.call('PEXPIRE', used_key, month_ttl)
end
local amount = redis.call('GET', reservation_key)
if amount then
  local remaining = redis.call('DECRBY', reserved_key, tonumber(amount))
  if remaining <= 0 then
    redis.call('DEL', reserved_key)
  end
  redis.call('DEL', reservation_key)
end
return 1
`)

// Budget keeps Redis reservation state and the authoritative MySQL usage ledger.
type Budget struct {
	rdb *redis.Client
	db  *gorm.DB
}

// New 构造 Budget。
func New(rdb *redis.Client, db ...*gorm.DB) *Budget {
	budget := &Budget{rdb: rdb}
	if len(db) > 0 {
		budget.db = db[0]
	}
	return budget
}

// MonthKey 当月计数 Key：budget:{keyID}:{YYYYMM}。
func MonthKey(keyID uint64) string {
	now := time.Now()
	return fmt.Sprintf("budget:%d:%04d%02d", keyID, now.Year(), int(now.Month()))
}

// monthEndTTLFor returns time remaining until the month's end, plus a one-minute buffer.
func monthEndTTLFor(monthKey string) time.Duration {
	month := monthKey[strings.LastIndex(monthKey, ":")+1:]
	start, err := time.ParseInLocation("200601", month, time.Local)
	if err != nil {
		return time.Minute
	}
	now := time.Now()
	ttl := start.AddDate(0, 1, 0).Sub(now) + time.Minute
	if ttl < time.Minute {
		return time.Minute
	}
	return ttl
}

// Exceeded 判断是否已超预算；返回 (是否超限, 当月已花费)。
func (b *Budget) Exceeded(ctx context.Context, keyID uint64, limit float64) (bool, float64, error) {
	spent, err := b.monthlySpent(ctx, keyID)
	if err != nil {
		return false, 0, err
	}
	return spent >= limit, spent, nil
}

// Reserve atomically reserves an estimated monthly cost against committed MySQL
// usage plus in-flight requests. Amounts are rounded up to micro-units.
func (b *Budget) Reserve(ctx context.Context, keyID uint64, requestID string, limit, estimatedCost float64, ttl time.Duration) (bool, error) {
	return b.ReserveForMonth(ctx, keyID, MonthKey(keyID), requestID, limit, estimatedCost, ttl)
}

// ReserveForMonth reserves cost against a specific request-month key.
func (b *Budget) ReserveForMonth(ctx context.Context, keyID uint64, monthKey, requestID string, limit, estimatedCost float64, ttl time.Duration) (bool, error) {
	if limit <= 0 || estimatedCost <= 0 {
		return true, nil
	}
	if b.rdb == nil {
		return false, errors.New("budget Redis client is unavailable")
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	spent, err := b.monthlySpentForMonth(ctx, keyID, monthKey)
	if err != nil {
		return false, err
	}
	const scale = 1_000_000.0
	baseMicro := int64(math.Round(spent * scale))
	limitMicro := int64(math.Floor(limit * scale))
	amountMicro := int64(math.Ceil(estimatedCost * scale))
	month := strings.TrimPrefix(monthKey, "budget:")
	keys := []string{
		"budget:reservation:" + month + ":" + requestID,
		monthKey,
		"budget:reserved:" + month,
	}
	result, err := reserveMonthlyBudget.Run(ctx, b.rdb, keys, baseMicro, limitMicro, amountMicro, ttl.Milliseconds(), monthEndTTLFor(monthKey).Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// Release removes a request's monthly budget reservation exactly once.
func (b *Budget) Release(ctx context.Context, keyID uint64, requestID string) error {
	return b.ReleaseForMonth(ctx, keyID, MonthKey(keyID), requestID)
}

// ReleaseForMonth releases a reservation created in a specific month.
func (b *Budget) ReleaseForMonth(ctx context.Context, keyID uint64, monthKey, requestID string) error {
	if b.rdb == nil {
		return errors.New("budget Redis client is unavailable")
	}
	month := strings.TrimPrefix(monthKey, "budget:")
	return releaseMonthlyBudget.Run(ctx, b.rdb, []string{
		"budget:reservation:" + month + ":" + requestID,
		"budget:reserved:" + month,
	}).Err()
}

// Settle applies actual committed cost and releases the estimate atomically.
func (b *Budget) Settle(ctx context.Context, keyID uint64, requestID string, actualCost float64) error {
	return b.SettleForMonth(ctx, keyID, MonthKey(keyID), requestID, actualCost)
}

// SettleForMonth commits cost and releases the reservation in a specific month.
func (b *Budget) SettleForMonth(ctx context.Context, keyID uint64, monthKey, requestID string, actualCost float64) error {
	if b.rdb == nil {
		return errors.New("budget Redis client is unavailable")
	}
	month := strings.TrimPrefix(monthKey, "budget:")
	const scale = 1_000_000.0
	actualMicro := int64(math.Round(actualCost * scale))
	return settleMonthlyBudget.Run(ctx, b.rdb, []string{
		monthKey,
		"budget:reservation:" + month + ":" + requestID,
		"budget:reserved:" + month,
	}, actualMicro, monthEndTTLFor(monthKey).Milliseconds()).Err()
}

func (b *Budget) monthlySpent(ctx context.Context, keyID uint64) (float64, error) {
	return b.monthlySpentForMonth(ctx, keyID, MonthKey(keyID))
}

func (b *Budget) monthlySpentForMonth(ctx context.Context, keyID uint64, monthKey string) (float64, error) {
	if b.db == nil {
		return 0, errors.New("monthly budget MySQL ledger is unavailable")
	}
	prefix := fmt.Sprintf("budget:%d:", keyID)
	month, ok := strings.CutPrefix(monthKey, prefix)
	if !ok {
		return 0, errors.New("invalid budget month key")
	}
	monthStart, err := time.ParseInLocation("200601", month, time.Local)
	if err != nil {
		return 0, errors.New("invalid budget month key")
	}
	nextMonth := monthStart.AddDate(0, 1, 0)
	var spent float64
	if err := b.db.WithContext(ctx).Model(&model.UsageLog{}).
		Select("COALESCE(SUM(cost), 0)").
		Where("api_key_id = ? AND created_at >= ? AND created_at < ?", keyID, monthStart, nextMonth).
		Scan(&spent).Error; err != nil {
		return 0, err
	}
	return spent, nil
}
