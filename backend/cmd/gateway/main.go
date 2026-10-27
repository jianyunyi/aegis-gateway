// AEGIS Gateway 入口。
// 支持 -migrate-only 仅执行数据库迁移后退出（供部署脚本使用）。
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/router"
	"aegis-gateway/internal/service"
	"aegis-gateway/migrations"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "run database migration and exit")
	flag.Parse()

	cfg := config.Load()
	var validationErr error
	if *migrateOnly {
		validationErr = cfg.ValidateMigration()
	} else {
		validationErr = cfg.Validate()
	}
	if validationErr != nil {
		slog.Error("unsafe or incomplete configuration", "error", validationErr)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if *migrateOnly {
		repo, err := repository.NewMySQL(cfg)
		if err != nil {
			slog.Error("init MySQL for migration failed", "error", err)
			os.Exit(1)
		}
		defer repo.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := migrations.Apply(ctx, repo.DB); err != nil {
			slog.Error("SQL migration failed", "error", err)
			os.Exit(1)
		}
		slog.Info("SQL migrations applied")
		return
	}

	repo, err := repository.New(cfg)
	if err != nil {
		slog.Error("init repository failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()

	if cfg.AutoMigrate {
		if err := repo.AutoMigrate(); err != nil {
			slog.Error("development auto migrate failed", "error", err)
			os.Exit(1)
		}
	}

	// 可选演示目录；首个管理员必须通过显式引导凭据创建，绝不生成示例 API Key。
	if cfg.SeedData {
		if err := service.Seed(repo, cfg.BootstrapAdminUser, cfg.BootstrapAdminPass); err != nil {
			slog.Error("seed failed", "error", err)
			os.Exit(1)
		}
		slog.Info("optional seed data initialized")
	}
	retentionCtx, retentionCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if removed, err := service.CleanupExpiredPromptData(retentionCtx, repo, cfg.PromptRetentionDays); err != nil {
		slog.Error("prompt retention cleanup failed")
	} else if removed > 0 {
		slog.Info("prompt retention cleanup completed", "removed_records", removed)
	}
	retentionCancel()

	r := router.New(cfg, repo)

	// 计费对账定时任务（goroutine，随进程生命周期）
	billingCtx, billingCancel := context.WithCancel(context.Background())
	defer billingCancel()
	service.NewBillingService(repo).StartTicker(billingCtx, cfg.BillingInterval)
	service.StartPromptRetention(billingCtx, repo, cfg.PromptRetentionDays)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		slog.Info("gateway listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// 优雅退出：等待 SIGINT/SIGTERM，10s 内完成在途请求
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	repo.Draining.Store(true)
	billingCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("gateway stopped")
}
