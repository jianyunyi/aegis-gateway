package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/repository"
)

// 以下为管理侧 REST 端点（/api/v1/admin）。M3 里程碑逐个实现，
// 当前为骨架占位。统一响应：{code, data, message}。

// AdminLogin M3 实现 JWT 签发。
func AdminLogin(repo *repository.Repository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"code":    50001,
			"message": "admin login: to be implemented in M3",
			"data":    nil,
		})
	}
}

// AdminStub 管理端占位实现。
func AdminStub(ep string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"code":    50001,
			"message": "admin endpoint /" + ep + ": to be implemented in M3",
			"data":    nil,
		})
	}
}
