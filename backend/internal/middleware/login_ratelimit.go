package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"aegis-gateway/internal/repository"
)

const (
	loginLimit       = 10
	loginWindow      = 5 * time.Minute
	defaultLoginTime = 500 * time.Millisecond
)

var loginAttemptScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// LoginRateLimit allows at most ten login attempts per client IP per five
// minutes, shared across gateway instances via Redis. Redis errors fail closed.
func LoginRateLimit(repo *repository.Repository, timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = defaultLoginTime
	}
	return func(c *gin.Context) {
		ip := c.ClientIP()
		digest := sha256.Sum256([]byte(ip))
		key := "auth:login:" + hex.EncodeToString(digest[:])
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		count, err := loginAttemptScript.Run(ctx, repo.Redis, []string{key}, loginWindow.Milliseconds()).Int()
		cancel()
		if err != nil {
			slog.Error("login rate limit unavailable", "error", err)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50301, "message": "登录保护暂不可用", "data": nil})
			return
		}
		if count > loginLimit {
			c.Header("Retry-After", "300")
			c.Set("result", "login_rate_limited")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 42901, "message": "登录尝试过于频繁，请稍后重试", "data": nil})
			return
		}
		c.Next()
	}
}
