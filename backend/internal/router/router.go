// Package router 组装 Gin 路由与中间件链。
package router

import (
	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/budget"
	"aegis-gateway/internal/config"
	"aegis-gateway/internal/handler"
	"aegis-gateway/internal/middleware"
	"aegis-gateway/internal/proxy"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/responsecache"
	"aegis-gateway/internal/routing"
	"aegis-gateway/internal/service"
)

// New 构建完整路由。
func New(cfg *config.Config, repo *repository.Repository) *gin.Engine {
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 聚合依赖（handler.Deps）
	d := &handler.Deps{
		Cfg:       cfg,
		Repo:      repo,
		Auth:      service.NewAuthService(repo, cfg.JWTSecret, cfg.JWTExpire),
		Keys:      service.NewKeyService(repo),
		Providers: service.NewProviderService(repo, cfg.JWTSecret),
		Models:    service.NewModelService(repo),
		Stats:     service.NewStatsService(repo),
		Billing:   service.NewBillingService(repo),
		Router:    routing.NewRouter(repo),
		Cache:     responsecache.New(repo.Redis, cfg.CacheTTL),
		Budget:    budget.New(repo.Redis),
		Upstream:  proxy.NewUpstreamClient(cfg.UpstreamTimeout),
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog())

	// 健康检查
	r.GET("/healthz", handler.Health)

	// 代理侧：OpenAI 兼容协议（Bearer ak_xxx）→ KeyAuth → RateLimit
	pg := r.Group("/v1")
	pg.Use(middleware.KeyAuth(repo), middleware.RateLimit(repo))
	{
		pg.POST("/chat/completions", handler.ChatCompletions(d))
		pg.POST("/completions", handler.Completions(d))
		pg.POST("/embeddings", handler.Embeddings(d))
		pg.GET("/models", handler.ProxyListModels(d))
	}

	// 管理侧：REST + JWT
	admin := r.Group("/api/v1/admin")
	{
		admin.POST("/auth/login", handler.AdminLogin(d))

		authed := admin.Group("")
		authed.Use(middleware.JWTAuth(cfg.JWTSecret))
		{
			authed.GET("/keys", handler.ListKeys(d))
			authed.POST("/keys", handler.CreateKey(d))
			authed.PUT("/keys/:id", handler.SetKeyStatus(d))
			authed.GET("/providers", handler.ListProviders(d))
			authed.POST("/providers", handler.CreateProvider(d))
			authed.GET("/models", handler.ListModels(d))
			authed.POST("/models", handler.CreateModel(d))

			// M3 计费与观测
			authed.GET("/stats/overview", handler.StatsOverview(d))
			authed.GET("/stats/trends", handler.StatsTrends(d))
			authed.GET("/logs", handler.ListLogs(d))
			authed.GET("/billing/daily", handler.BillingDaily(d))
			authed.POST("/billing/reconcile", handler.BillingReconcile(d))

			// M5 评测飞轮实现
			authed.POST("/evals/datasets", handler.AdminStub("evals/datasets"))
			authed.GET("/evals/datasets", handler.AdminStub("evals/datasets"))
			authed.POST("/evals/runs", handler.AdminStub("evals/runs"))
			authed.GET("/evals/runs/:id/report", handler.AdminStub("evals/runs/report"))
		}
	}

	return r
}
