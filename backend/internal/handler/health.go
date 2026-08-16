package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health 健康检查端点，M1 演示点。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "aegis-gateway",
		"version": "v0.1.0-m1",
	})
}
