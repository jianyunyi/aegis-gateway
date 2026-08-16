// Package repository 统一管理 MySQL 与 Redis 连接与迁移。
package repository

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/model"
)

// Repository 聚合数据访问入口。
type Repository struct {
	DB    *gorm.DB
	Redis *redis.Client
}

// New 初始化 MySQL 与 Redis 连接并做连通性检查。
func New(cfg *config.Config) (*Repository, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	return &Repository{DB: db, Redis: rdb}, nil
}

// AutoMigrate 同步表结构（演示环境使用；生产环境应走 migrations/ SQL 迁移）。
func (r *Repository) AutoMigrate() error {
	return r.DB.AutoMigrate(
		&model.User{},
		&model.ApiKey{},
		&model.Provider{},
		&model.Model{},
		&model.UsageLog{},
		&model.BillingDaily{},
		&model.EvalDataset{},
		&model.EvalSample{},
		&model.EvalRun{},
	)
}

// Close 释放连接资源。
func (r *Repository) Close() {
	if r.DB != nil {
		if sqlDB, err := r.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if r.Redis != nil {
		_ = r.Redis.Close()
	}
}
