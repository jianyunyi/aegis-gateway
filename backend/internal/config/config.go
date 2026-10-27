// Package config 统一加载网关配置（12-Factor：环境变量驱动）。
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultMaxRequestBodyBytes = 2 << 20 // 2 MiB

// Config 网关全部配置项。
type Config struct {
	Env                 string        // dev / prod
	HTTPAddr            string        // 监听地址
	MySQLDSN            string        // MySQL 连接串
	RedisAddr           string        // Redis 地址 host:port
	RedisPass           string        // Redis 地址密码
	AutoMigrate         bool          // 仅开发环境可自动同步 schema
	JWTSecret           string        // 管理后台 JWT 签名与提供商凭证加密密钥
	JWTExpire           time.Duration // JWT 有效期
	UpstreamTimeout     time.Duration // 上游调用超时
	SeedData            bool          // 显式写入演示目录和管理员
	BootstrapAdminUser  string        // 首次启用种子时创建管理员的用户名
	BootstrapAdminPass  string        // 首次启用种子时创建管理员的密码
	CapturePrompts      bool          // 显式允许保存脱敏 prompt 用于评测采样
	PromptRetentionDays int           // 含 prompt 的日志/评测结果保留天数
	BillingInterval     time.Duration // 计费对账定时任务间隔（0=禁用）
	CacheTTL            time.Duration // 请求缓存 TTL
	RateLimitFailOpen   bool
	RateLimitTimeout    time.Duration
	ReadinessTimeout    time.Duration
	MaxRequestBodyBytes int64
	CORSAllowedOrigins  []string
}

// Load 从环境变量加载配置。生产环境没有任何内置凭据或公开 JWT 密钥。
func Load() *Config {
	env := getEnv("APP_ENV", "dev")
	dev := strings.EqualFold(env, "dev") || strings.EqualFold(env, "development") || strings.EqualFold(env, "test")
	jwtDefault := ""
	if dev {
		jwtDefault = "dev-only-secret-change-before-deploy"
	}
	origins := getCSV("CORS_ALLOWED_ORIGINS")
	if len(origins) == 0 && dev {
		origins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}
	return &Config{
		Env:                 env,
		HTTPAddr:            getEnv("HTTP_ADDR", ":8081"),
		MySQLDSN:            getEnv("MYSQL_DSN", ""),
		RedisAddr:           getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPass:           getEnv("REDIS_PASSWORD", ""),
		AutoMigrate:         getBool("AUTO_MIGRATE", dev),
		JWTSecret:           getEnv("JWT_SECRET", jwtDefault),
		JWTExpire:           getDuration("JWT_EXPIRE_MINUTES", 12*60, time.Minute, false),
		UpstreamTimeout:     getDuration("UPSTREAM_TIMEOUT_SECONDS", 180, time.Second, false),
		SeedData:            getBool("SEED_DATA", false),
		BootstrapAdminUser:  getEnv("BOOTSTRAP_ADMIN_USERNAME", ""),
		BootstrapAdminPass:  getEnv("BOOTSTRAP_ADMIN_PASSWORD", ""),
		CapturePrompts:      getBool("PROMPT_CAPTURE_ENABLED", false),
		PromptRetentionDays: getInt("PROMPT_RETENTION_DAYS", 30, 1, 3650),
		BillingInterval:     getDuration("BILLING_INTERVAL_MINUTES", 5, time.Minute, true),
		CacheTTL:            getSeconds("CACHE_TTL_SECONDS", 600),
		RateLimitFailOpen:   getBool("RATE_LIMIT_FAIL_OPEN", false),
		RateLimitTimeout:    getDuration("RATE_LIMIT_TIMEOUT_MS", 500, time.Millisecond, false),
		ReadinessTimeout:    getDuration("READINESS_TIMEOUT_MS", 1000, time.Millisecond, false),
		MaxRequestBodyBytes: int64(getInt("MAX_REQUEST_BODY_BYTES", defaultMaxRequestBodyBytes, 1024, 16<<20)),
		CORSAllowedOrigins:  origins,
	}
}

// Validate rejects unsafe production configuration and incomplete explicit bootstrap settings.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("configuration is nil")
	}
	if c.MaxRequestBodyBytes < 1024 || c.MaxRequestBodyBytes > 16<<20 {
		return errors.New("MAX_REQUEST_BODY_BYTES must be between 1024 and 16777216")
	}
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "dev", "development", "test":
		if strings.TrimSpace(c.MySQLDSN) == "" {
			return errors.New("MYSQL_DSN is required")
		}
		return nil
	case "prod", "production":
	default:
		return errors.New("APP_ENV must be dev, test, or prod")
	}
	if len([]byte(c.JWTSecret)) < 32 || c.JWTSecret == "dev-only-secret-change-before-deploy" {
		return errors.New("JWT_SECRET must be explicitly set to at least 32 bytes in production")
	}
	if c.RedisPass == "" {
		return errors.New("REDIS_PASSWORD is required in production")
	}
	dsnCredentials := strings.SplitN(strings.SplitN(c.MySQLDSN, "@", 2)[0], ":", 2)
	if len(dsnCredentials) != 2 || strings.TrimSpace(dsnCredentials[0]) == "" || dsnCredentials[1] == "" {
		return errors.New("MYSQL_DSN must include dedicated production username and password")
	}
	if strings.EqualFold(dsnCredentials[0], "root") {
		return errors.New("MYSQL_DSN must use a dedicated non-root account in production")
	}
	if c.AutoMigrate {
		return errors.New("AUTO_MIGRATE must be false in production; use the versioned SQL migrations")
	}
	if c.RateLimitFailOpen {
		return errors.New("RATE_LIMIT_FAIL_OPEN must be false in production")
	}
	if c.SeedData && (strings.TrimSpace(c.BootstrapAdminUser) == "" || len([]byte(c.BootstrapAdminPass)) < 12) {
		return errors.New("SEED_DATA requires BOOTSTRAP_ADMIN_USERNAME and a BOOTSTRAP_ADMIN_PASSWORD of at least 12 bytes")
	}
	return nil
}

// IsProduction identifies supported production environment labels.
func (c *Config) IsProduction() bool {
	return c != nil && (strings.EqualFold(c.Env, "prod") || strings.EqualFold(c.Env, "production"))
}

// ValidateMigration checks only configuration required by the MySQL-only migration command.
func (c *Config) ValidateMigration() error {
	if c == nil {
		return errors.New("configuration is nil")
	}
	mode := strings.ToLower(strings.TrimSpace(c.Env))
	if mode != "dev" && mode != "development" && mode != "test" && mode != "prod" && mode != "production" {
		return errors.New("APP_ENV must be dev, test, or prod")
	}
	if strings.TrimSpace(c.MySQLDSN) == "" {
		return errors.New("MYSQL_DSN is required for migrations")
	}
	if c.IsProduction() {
		credentials := strings.SplitN(strings.SplitN(c.MySQLDSN, "@", 2)[0], ":", 2)
		if len(credentials) != 2 || strings.TrimSpace(credentials[0]) == "" || credentials[1] == "" || strings.EqualFold(credentials[0], "root") {
			return errors.New("production migrations require a dedicated non-root MySQL account and password")
		}
	}
	return nil
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

func getInt(key string, def, min, max int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= min && n <= max {
			return n
		}
	}
	return def
}

func getCSV(key string) []string {
	var values []string
	for _, value := range strings.Split(os.Getenv(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
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

// getSeconds reads seconds and converts them to a time.Duration.
func getSeconds(key string, defSeconds int) time.Duration {
	return getDuration(key, defSeconds, time.Second, false)
}
