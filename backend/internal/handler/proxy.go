package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/repository"
)

// 以下为代理侧（OpenAI 兼容协议）端点。M2 里程碑实现代理与流式转发，
// 当前为骨架占位，统一返回 OpenAI 格式错误。

func notImplemented(c *gin.Context, ep string) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": gin.H{
			"message": ep + ": to be implemented in M2",
			"type":    "not_implemented",
		},
	})
}

// ChatCompletions 处理 POST /v1/chat/completions。
func ChatCompletions(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		notImplemented(c, "chat/completions")
	}
}

// Completions 处理 POST /v1/completions。
func Completions(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		notImplemented(c, "completions")
	}
}

// Embeddings 处理 POST /v1/embeddings。
func Embeddings(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		notImplemented(c, "embeddings")
	}
}

// ListModels 处理 GET /v1/models，返回网关可用模型目录。
func ListModels(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		notImplemented(c, "models")
	}
}
