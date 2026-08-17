// Package responsecache 请求级响应缓存：相同请求（model+参数+消息 完全一致）直接命中，
// 不重复调用上游，成本归零（计费按 0 元记录，token 仍计入统计）。
package responsecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache 响应缓存（Redis 存储完整响应 JSON）。
type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

// New 构造缓存。
func New(rdb *redis.Client, ttl time.Duration) *Cache {
	return &Cache{rdb: rdb, ttl: ttl}
}

// KeyFor 由请求体（含 model/messages/参数）生成缓存键，完全一致才命中。
func KeyFor(body []byte) string {
	sum := sha256.Sum256(body)
	return "pc:" + hex.EncodeToString(sum[:])
}

// Get 读取缓存；命中返回响应体。
func (c *Cache) Get(ctx context.Context, key string) (string, bool) {
	v, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return v, true
}

// Set 写入缓存（TTL 由配置决定）。
func (c *Cache) Set(ctx context.Context, key, body string) error {
	return c.rdb.Set(ctx, key, body, c.ttl).Err()
}
