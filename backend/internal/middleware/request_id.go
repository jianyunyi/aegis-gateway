// Package middleware 网关中间件链：RequestID → AccessLog → KeyAuth → RateLimit → Sanitize → Route。
package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

// RequestIDKey 在 gin.Context 中存放 request_id 的键。
const RequestIDKey = "request_id"

// RequestID 为每个请求生成/透传 request_id，贯穿全链路。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			rid = hex.EncodeToString(b)
		}
		c.Set(RequestIDKey, rid)
		c.Header("x-request-id", rid)
		c.Next()
	}
}
