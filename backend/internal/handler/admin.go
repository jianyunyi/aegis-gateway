package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminStub 管理端占位实现（M3/M5 里程碑逐个替换为真实实现）。
func AdminStub(ep string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"code":    50001,
			"message": "admin endpoint /" + ep + ": to be implemented in M3",
			"data":    nil,
		})
	}
}
