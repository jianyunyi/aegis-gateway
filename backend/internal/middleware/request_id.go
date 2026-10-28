// Package middleware 网关中间件链：RequestID → AccessLog → KeyAuth → RateLimit → Route。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestIDKey 在 gin.Context 中存放网关生成的内部 request_id。
const RequestIDKey = "request_id"

// RequestID assigns a cryptographically random internal ID per request. Client
// supplied X-Request-ID values are deliberately ignored so they cannot collide
// with the unique usage ledger key; the gateway's value is returned to callers.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "request id generation failed"}})
			return
		}
		rid := hex.EncodeToString(bytes[:])
		c.Set(RequestIDKey, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}
