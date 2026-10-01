package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// rlScript 令牌桶（Redis Lua 原子执行，避免并发读-改-写竞态）：
// KEYS[1]=tokens, KEYS[2]=最近补充时间
// ARGV[1]=rate(每秒补充), ARGV[2]=burst(桶容量), ARGV[3]=now(ms)
// 返回 1=放行, 0=拒绝。
const rlScript = `
local tokens = tonumber(redis.call('GET', KEYS[1]) or ARGV[2])
local last = tonumber(redis.call('GET', KEYS[2]) or ARGV[3])
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
tokens = math.min(burst, tokens + (now - last) / 1000.0 * rate)
if tokens < 1 then
  return 0
end
local ttl = math.ceil(burst / rate * 1000) + 1000
redis.call('SET', KEYS[1], tokens - 1, 'PX', ttl)
redis.call('SET', KEYS[2], now, 'PX', ttl)
return 1`

// RateLimit 按 API Key 做令牌桶限流（在 KeyAuth 之后执行）。
// Redis failures use the configured policy; default is fail-closed (503).
func RateLimit(repo *repository.Repository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := c.MustGet(CtxAPIKey).(*model.ApiKey)
		if !ok {
			c.Next()
			return
		}
		if key.RPSLimit <= 0 {
			c.Next() // 0 表示不限
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.RateLimitTimeout)
		defer cancel()
		id := strconv.FormatUint(key.ID, 10)
		allow, err := repo.Redis.Eval(ctx, rlScript,
			[]string{"rl:{" + id + "}:tokens", "rl:{" + id + "}:ts"},
			key.RPSLimit, key.Burst, time.Now().UnixMilli()).Int()
		if err != nil {
			c.Set("result", "rate_limit_dependency_error")
			slog.Error("rate_limit_dependency_error", "request_id", c.GetString(RequestIDKey), "fail_open", cfg.RateLimitFailOpen, "error", err)
			if cfg.RateLimitFailOpen {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "rate limiter unavailable", "type": "rate_limit_unavailable"}})
			return
		}
		if allow != 1 {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"message": "rate limit exceeded", "type": "rate_limit_error"},
			})
			return
		}
		c.Next()
	}
}
