// Package budget 月度预算控制：MySQL 用量流水为权威账本，Redis Lua 原子管理并发成本预留。
package budget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
local expiry_key = KEYS[4]
local amounts_key = KEYS[5]
local ttl = tonumber(ARGV[4])
local month_ttl = tonumber(ARGV[5])
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)

local expired = redis.call('ZRANGEBYSCORE', expiry_key, '-inf', now_ms)
for _, expired_booking in ipairs(expired) do
  local expired_amount = redis.call('HGET', amounts_key, expired_booking)
  if expired_amount then
    local remaining = redis.call('DECRBY', reserved_key, expired_amount)
    if tonumber(remaining) <= 0 then
      redis.call('DEL', reserved_key)
    end
  end
  redis.call('DEL', expired_booking)
  redis.call('HDEL', amounts_key, expired_booking)
  redis.call('ZREM', expiry_key, expired_booking)
end
if redis.call('ZCARD', expiry_key) == 0 then
  redis.call('DEL', expiry_key, amounts_key)
end

if redis.call('EXISTS', booking) == 1 then
  return 1
end
local base = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local amount = ARGV[3]
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
if used + reserved + tonumber(amount) > limit then
  return 0
end
redis.call('INCRBY', reserved_key, amount)
redis.call('SET', booking, amount, 'PX', ttl)
local expiry_now = redis.call('TIME')
local expiry_ms = tonumber(expiry_now[1]) * 1000 + math.floor(tonumber(expiry_now[2]) / 1000) + redis.call('PTTL', booking)
redis.call('HSET', amounts_key, booking, amount)
redis.call('ZADD', expiry_key, expiry_ms, booking)

local function extend_ttl(key, desired)
  if redis.call('PTTL', key) < desired then
    redis.call('PEXPIRE', key, desired)
  end
end
local tracking_ttl = math.max(ttl * 2, month_ttl)
extend_ttl(reserved_key, tracking_ttl)
extend_ttl(expiry_key, tracking_ttl)
extend_ttl(amounts_key, tracking_ttl)
return 1
`)

var releaseMonthlyBudget = redis.NewScript(`
local amount = redis.call('GET', KEYS[1])
if not amount then
  amount = redis.call('HGET', KEYS[4], KEYS[1])
end
if amount then
  local remaining = redis.call('DECRBY', KEYS[2], amount)
  if tonumber(remaining) <= 0 then
    redis.call('DEL', KEYS[2])
  end
end
redis.call('DEL', KEYS[1])
redis.call('HDEL', KEYS[4], KEYS[1])
redis.call('ZREM', KEYS[3], KEYS[1])
if redis.call('ZCARD', KEYS[3]) == 0 then
  redis.call('DEL', KEYS[3], KEYS[4])
end
return amount and 1 or 0
`)

var settleMonthlyBudget = redis.NewScript(`
local used_key = KEYS[1]
local reservation_key = KEYS[2]
local reserved_key = KEYS[3]
local expiry_key = KEYS[4]
local amounts_key = KEYS[5]
local committed = ARGV[1]
local month_ttl = tonumber(ARGV[2])
redis.call('SET', used_key, committed, 'PX', month_ttl)
local amount = redis.call('GET', reservation_key)
if not amount then
  amount = redis.call('HGET', amounts_key, reservation_key)
end
if amount then
  local remaining = redis.call('DECRBY', reserved_key, amount)
  if tonumber(remaining) <= 0 then
    redis.call('DEL', reserved_key)
  end
end
redis.call('DEL', reservation_key)
redis.call('HDEL', amounts_key, reservation_key)
redis.call('ZREM', expiry_key, reservation_key)
if redis.call('ZCARD', expiry_key) == 0 then
  redis.call('DEL', expiry_key, amounts_key)
end
return 1
`)

var unlockMonthlyBudget = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

const monthlyBudgetLockTTL = 30 * time.Second

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

func (b *Budget) withMonthLock(ctx context.Context, monthKey string, fn func() error) error {
	if b.rdb == nil {
		return errors.New("budget Redis client is unavailable")
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	token := hex.EncodeToString(secret)
	lockKey := "budget:month-lock:" + strings.TrimPrefix(monthKey, "budget:")
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		acquired, err := b.rdb.SetNX(ctx, lockKey, token, monthlyBudgetLockTTL).Result()
		if err != nil {
			return err
		}
		if acquired {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	fnErr := fn()
	unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlockErr := unlockMonthlyBudget.Run(unlockCtx, b.rdb, []string{lockKey}, token).Err()
	if fnErr != nil {
		return fnErr
	}
	return unlockErr
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
	var allowed bool
	err := b.withMonthLock(ctx, monthKey, func() error {
		spent, err := b.monthlySpentForMonth(ctx, keyID, monthKey)
		if err != nil {
			return err
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
			"budget:reservation-expiry:" + month,
			"budget:reservation-amounts:" + month,
		}
		result, err := reserveMonthlyBudget.Run(ctx, b.rdb, keys, baseMicro, limitMicro, amountMicro, ttl.Milliseconds(), monthEndTTLFor(monthKey).Milliseconds()).Int()
		allowed = result == 1
		return err
	})
	return allowed, err
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
		"budget:reservation-expiry:" + month,
		"budget:reservation-amounts:" + month,
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
	return b.withMonthLock(ctx, monthKey, func() error {
		return b.settleForMonthLocked(ctx, keyID, monthKey, requestID, actualCost)
	})
}

// CommitUsageForMonth serializes the durable usage write with budget projection
// and reservation release, so SQL usage is never counted alongside its hold.
func (b *Budget) CommitUsageForMonth(ctx context.Context, keyID uint64, monthKey, requestID string, actualCost float64, persist func() error) error {
	return b.withMonthLock(ctx, monthKey, func() error {
		if err := persist(); err != nil {
			return err
		}
		return b.settleForMonthLocked(ctx, keyID, monthKey, requestID, actualCost)
	})
}

func (b *Budget) settleForMonthLocked(ctx context.Context, keyID uint64, monthKey, requestID string, actualCost float64) error {
	if b.rdb == nil {
		return errors.New("budget Redis client is unavailable")
	}
	spent, err := b.monthlySpentForMonth(ctx, keyID, monthKey)
	if err != nil {
		return err
	}
	month := strings.TrimPrefix(monthKey, "budget:")
	const scale = 1_000_000.0
	committedMicro := int64(math.Round(spent * scale))
	actualMicro := int64(math.Round(actualCost * scale))
	if actualMicro > committedMicro {
		committedMicro = actualMicro
	}
	return settleMonthlyBudget.Run(ctx, b.rdb, []string{
		monthKey,
		"budget:reservation:" + month + ":" + requestID,
		"budget:reserved:" + month,
		"budget:reservation-expiry:" + month,
		"budget:reservation-amounts:" + month,
	}, committedMicro, monthEndTTLFor(monthKey).Milliseconds()).Err()
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
