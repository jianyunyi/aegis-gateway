// Package config 统一加载网关配置（12-Factor：环境变量驱动）。
package config

import (
	"os"
	"strconv"
	"time"
)

// Config 网关全部配置项。
type Config struct {
	Env               string        // dev / prod
	HTTPAddr          string        // 监听地址
	MySQLDSN          string        // MySQL 连接串
	RedisAddr         string        // Redis 地址 host:port
	RedisPass         string        // Redis 密码（可为空）
	AutoMigrate       bool          // 启动时自动迁移表结构（演示环境开启）
	JWTSecret         string        // 管理后台 JWT 签名密钥
	JWTExpire         time.Duration // JWT 有效期
	UpstreamTimeout   time.Duration // 上游调用超时
	SeedData          bool          // 首次启动写入演示数据
	BillingInterval   time.Duration // 计费对账定时任务间隔（0=禁用）
	CacheTTL          time.Duration // 请求缓存 TTL
	RateLimitFailOpen bool
	RateLimitTimeout  time.Duration
	ReadinessTimeout  time.Duration
}

// Load 从环境变量加载配置，未设置时使用本地开发默认值。
func Load() *Config {
	return &Config{
		Env:               getEnv("APP_ENV", "dev"),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8081"),
		MySQLDSN:          getEnv("MYSQL_DSN", "root:root@tcp(127.0.0.1:3303)/aegis?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:         getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPass:         getEnv("REDIS_PASSWORD", ""),
		AutoMigrate:       getBool("AUTO_MIGRATE", true),
		JWTSecret:         getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTExpire:         getDuration("JWT_EXPIRE_MINUTES", 12*60, time.Minute, false),
		UpstreamTimeout:   getDuration("UPSTREAM_TIMEOUT_SECONDS", 180, time.Second, false),
		SeedData:          getBool("SEED_DATA", true),
		BillingInterval:   getDuration("BILLING_INTERVAL_MINUTES", 5, time.Minute, true),
		CacheTTL:          getSeconds("CACHE_TTL_SECONDS", 600),
		RateLimitFailOpen: getBool("RATE_LIMIT_FAIL_OPEN", false),
		RateLimitTimeout:  getDuration("RATE_LIMIT_TIMEOUT_MS", 500, time.Millisecond, false),
		ReadinessTimeout:  getDuration("READINESS_TIMEOUT_MS", 1000, time.Millisecond, false),
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

// getDuration reads an explicit unit; zero is accepted only for optional jobs.
func getDuration(key string, fallback int, unit time.Duration, allowZero bool) time.Duration {
	v := fallback
	if s := os.Getenv(key); s != "" {
		if n, err := strconv.Atoi(s); err == nil && (n > 0 || (allowZero && n == 0)) && int64(n) <= int64((1<<63-1)/unit) {
			v = n
		}
	}
	return time.Duration(v) * unit
}

// getSeconds 读取秒数（int），转换为 time.Duration。
func getSeconds(key string, defSeconds int) time.Duration {
	return getDuration(key, defSeconds, time.Second, false)
}
