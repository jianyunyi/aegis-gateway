package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// CtxUserID / CtxUsername 存放解析后的 JWT 声明。
const (
	CtxUserID   = "user_id"
	CtxUsername = "username"
)

// JWTAuth 管理侧鉴权中间件（HS256，校验签名与有效期）。
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			abortAdmin(c, http.StatusUnauthorized, 40101, "未登录")
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !token.Valid {
			abortAdmin(c, http.StatusUnauthorized, 40101, "登录已过期，请重新登录")
			return
		}
		c.Set(CtxUserID, claims["uid"])
		c.Set(CtxUsername, claims["username"])
		c.Next()
	}
}

func abortAdmin(c *gin.Context, httpStatus, code int, msg string) {
	c.AbortWithStatusJSON(httpStatus, gin.H{"code": code, "message": msg, "data": nil})
}
