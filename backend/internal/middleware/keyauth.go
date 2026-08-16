package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// CtxAPIKey 在 gin.Context 中存放鉴权通过后的 ApiKey 对象。
const CtxAPIKey = "api_key"

// KeyAuth 校验调用方 API Key（Bearer ak_xxx）。
// 安全设计：库中仅存 SHA-256 哈希（ADR-007），此处哈希比对。
// M2 优化项：命中后缓存 key 状态到 Redis，避免每次查库。
func KeyAuth(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			abortUnauthorized(c, "missing bearer token")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if !strings.HasPrefix(token, "ak_") {
			abortUnauthorized(c, "invalid key format")
			return
		}

		sum := sha256.Sum256([]byte(token))
		hash := hex.EncodeToString(sum[:])

		var key model.ApiKey
		if err := repo.DB.Where("key_hash = ?", hash).First(&key).Error; err != nil {
			abortUnauthorized(c, "invalid api key")
			return
		}
		if key.Status != 1 {
			abortUnauthorized(c, "api key disabled")
			return
		}
		if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
			abortUnauthorized(c, "api key expired")
			return
		}
		c.Set(CtxAPIKey, &key)
		c.Next()
	}
}

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{"message": msg, "type": "authentication_error"},
	})
}
