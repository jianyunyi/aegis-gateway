package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"aegis-gateway/internal/repository"
)

const (
	loginIPLimit     = 10
	loginUserIPLimit = 5
	loginWindow      = 5 * time.Minute
	defaultLoginTime = 500 * time.Millisecond
)

// One script updates every applicable dimension in one Redis transaction. The
// first increment on each key installs the window, so all gateway instances
// observe the same fixed five-minute buckets.
var loginAttemptScript = redis.NewScript(`
local counts = {}
for i, key in ipairs(KEYS) do
  local count = redis.call('INCR', key)
  if count == 1 then
    redis.call('PEXPIRE', key, ARGV[1])
  end
  counts[i] = count
end
return counts
`)

func loginKey(prefix, value string) string {
	digest := sha256.Sum256([]byte(value))
	return prefix + hex.EncodeToString(digest[:])
}

func normalizedLoginUsername(body []byte) (string, bool) {
	var payload struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	username := strings.ToLower(strings.TrimSpace(payload.Username))
	if username == "" {
		return "", false
	}
	return username, true
}

// LoginRateLimit allows at most ten login attempts per client IP and five
// attempts per IP and normalized username within five minutes. It deliberately
// avoids a global username bucket so remote attackers cannot lock out an
// account by rotating source IPs. Redis errors fail closed.
func LoginRateLimit(repo *repository.Repository, timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = defaultLoginTime
	}
	return func(c *gin.Context) {
		body, readErr := loginReadAndRestoreBody(c.Request)
		if readErr != nil {
			// Preserve the handler's existing body-validation behavior as much as
			// possible while still applying the IP dimension.
			body = nil
		}

		ip := c.ClientIP()
		keys := []string{loginKey("auth:login:ip:", ip)}
		limits := []int{loginIPLimit}
		if username, ok := normalizedLoginUsername(body); ok {
			keys = append(keys,
				loginKey("auth:login:ip-user:", ip+"\x00"+username),
			)
			limits = append(limits, loginUserIPLimit)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		var counts []int
		var err error
		if repo == nil || repo.Redis == nil {
			err = redis.ErrClosed
		} else {
			result, runErr := loginAttemptScript.Run(ctx, repo.Redis, keys, loginWindow.Milliseconds()).Result()
			err = runErr
			if err == nil {
				counts, err = loginCounts(result)
			}
		}
		cancel()
		if err != nil {
			slog.Error("login rate limit unavailable", "error", err)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50301, "message": "登录保护暂不可用", "data": nil})
			return
		}
		for i, count := range counts {
			if count > limits[i] {
				c.Header("Retry-After", "300")
				c.Set("result", "login_rate_limited")
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 42901, "message": "登录尝试过于频繁，请稍后重试", "data": nil})
				return
			}
		}
		c.Next()
	}
}

func loginCounts(result interface{}) ([]int, error) {
	values, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected login rate limit result type %T", result)
	}
	counts := make([]int, len(values))
	for i, value := range values {
		count, ok := value.(int64)
		if !ok {
			return nil, fmt.Errorf("unexpected login rate limit count type %T", value)
		}
		counts[i] = int(count)
	}
	return counts, nil
}

func loginReadAndRestoreBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	return body, err
}
