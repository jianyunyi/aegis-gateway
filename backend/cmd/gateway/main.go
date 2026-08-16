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
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "run database migration and exit")
	flag.Parse()

	cfg := config.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	repo, err := repository.New(cfg)
	if err != nil {
		slog.Error("init repository failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()

	if cfg.AutoMigrate || *migrateOnly {
		if err := repo.AutoMigrate(); err != nil {
			slog.Error("auto migrate failed", "error", err)
			os.Exit(1)
		}
		slog.Info("database migrated")
		if *migrateOnly {
			return
		}
	}

	r := router.New(cfg, repo)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("gateway stopped")
}
