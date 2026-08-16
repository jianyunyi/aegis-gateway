// Package router 组装 Gin 路由与中间件链。
package router

import (
	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/handler"
	"aegis-gateway/internal/middleware"
	"aegis-gateway/internal/repository"
)

// New 构建完整路由。
func New(cfg *config.Config, repo *repository.Repository) *gin.Engine {
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog())

	// 健康检查
	r.GET("/healthz", handler.Health)

	// 代理侧：OpenAI 兼容协议（Bearer ak_xxx）
	proxy := r.Group("/v1")
	proxy.Use(middleware.KeyAuth(repo))
	{
		proxy.POST("/chat/completions", handler.ChatCompletions(repo))
		proxy.POST("/completions", handler.Completions(repo))
		proxy.POST("/embeddings", handler.Embeddings(repo))
		proxy.GET("/models", handler.ListModels(repo))
	}

	// 管理侧：REST + JWT（M3 里程碑接入 JWTAuth 中间件）
	admin := r.Group("/api/v1/admin")
	{
		admin.POST("/auth/login", handler.AdminLogin(repo, cfg))
		admin.GET("/stats/overview", handler.AdminStub("stats/overview"))
		admin.GET("/stats/trends", handler.AdminStub("stats/trends"))
		admin.GET("/logs", handler.AdminStub("logs"))
		admin.GET("/billing/daily", handler.AdminStub("billing/daily"))
		admin.GET("/keys", handler.AdminStub("keys"))
		admin.POST("/keys", handler.AdminStub("keys"))
		admin.GET("/providers", handler.AdminStub("providers"))
		admin.POST("/providers", handler.AdminStub("providers"))
		admin.GET("/models", handler.AdminStub("models"))
	}

	return r
}
