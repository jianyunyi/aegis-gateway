// Package budget 月度预算控制：按 Key 累计当月费用（Redis INCRBYFLOAT），
// 超限时由网关自动降级到便宜模型并告警（PRD Epic-6）。
package budget

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrParse 计数解析失败。
var ErrParse = errors.New("budget counter parse error")

// Budget 预算计数（Redis）。
type Budget struct {
	rdb *redis.Client
}

// New 构造 Budget。
func New(rdb *redis.Client) *Budget {
	return &Budget{rdb: rdb}
}

// MonthKey 当月计数 Key：budget:{keyID}:{YYYYMM}。
func MonthKey(keyID uint64) string {
	now := time.Now()
	return fmt.Sprintf("budget:%d:%04d%02d", keyID, now.Year(), int(now.Month()))
}

// monthEndTTL 距月末的剩余时间（含 1 分钟缓冲），用于 Key 自动过期。
func monthEndTTL() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	return next.Sub(now) + time.Minute
}

// Spent 当月已花费。
func (b *Budget) Spent(ctx context.Context, keyID uint64) (float64, error) {
	v, err := b.rdb.Get(ctx, MonthKey(keyID)).Float64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

// Add 累计费用并设置月末过期。
func (b *Budget) Add(ctx context.Context, keyID uint64, cost float64) error {
	if cost <= 0 {
		return nil
	}
	key := MonthKey(keyID)
	if err := b.rdb.IncrByFloat(ctx, key, cost).Err(); err != nil {
		return err
	}
	return b.rdb.Expire(ctx, key, monthEndTTL()).Err()
}

// Exceeded 判断是否已超预算；返回 (是否超限, 当月已花费)。
func (b *Budget) Exceeded(ctx context.Context, keyID uint64, limit float64) (bool, float64, error) {
	spent, err := b.Spent(ctx, keyID)
	if err != nil {
		return false, 0, err
	}
	return spent >= limit, spent, nil
}

// FormatFloat 辅助格式化（避免直接引 strconv 造成未使用告警）。
func FormatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 4, 64)
}
