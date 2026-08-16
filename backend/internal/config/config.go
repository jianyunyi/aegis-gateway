// Package config 统一加载网关配置（12-Factor：环境变量驱动）。
package config

import (
	"os"
	"strconv"
)

// Config 网关全部配置项。
type Config struct {
	Env         string // dev / prod
	HTTPAddr    string // 监听地址
	MySQLDSN    string // MySQL 连接串
	RedisAddr   string // Redis 地址 host:port
	RedisPass   string // Redis 密码（可为空）
	AutoMigrate bool   // 启动时自动迁移表结构（演示环境开启）
	JWTSecret   string // 管理后台 JWT 签名密钥
}

// Load 从环境变量加载配置，未设置时使用本地开发默认值。
func Load() *Config {
	return &Config{
		Env:         getEnv("APP_ENV", "dev"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		MySQLDSN:    getEnv("MYSQL_DSN", "root:root@tcp(127.0.0.1:3306)/aegis?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:   getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPass:   getEnv("REDIS_PASSWORD", ""),
		AutoMigrate: getBool("AUTO_MIGRATE", true),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-change-me"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
