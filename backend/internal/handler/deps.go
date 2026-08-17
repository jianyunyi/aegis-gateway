package handler

import (
	"aegis-gateway/internal/budget"
	"aegis-gateway/internal/config"
	"aegis-gateway/internal/proxy"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/responsecache"
	"aegis-gateway/internal/routing"
	"aegis-gateway/internal/service"
)

// Deps 聚合 handler 依赖，统一由 router 组装。
type Deps struct {
	Cfg       *config.Config
	Repo      *repository.Repository
	Auth      *service.AuthService
	Keys      *service.KeyService
	Providers *service.ProviderService
	Models    *service.ModelService
	Stats     *service.StatsService
	Billing   *service.BillingService
	Router    *routing.Router
	Cache     *responsecache.Cache
	Budget    *budget.Budget
	Upstream  *proxy.UpstreamClient
}
