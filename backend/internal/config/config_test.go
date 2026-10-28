package config

import (
	"strings"
	"testing"
	"time"
)

func TestDurationUnitsAndDisable(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"", 5 * time.Minute}, {"0", 0}, {"2", 2 * time.Minute}, {"-1", 5 * time.Minute}, {"bad", 5 * time.Minute}, {"9223372036854775807", 5 * time.Minute}} {
		t.Run("billing_"+tc.value, func(t *testing.T) {
			t.Setenv("BILLING_INTERVAL_MINUTES", tc.value)
			if got := Load().BillingInterval; got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	t.Setenv("UPSTREAM_TIMEOUT_SECONDS", "1")
	t.Setenv("JWT_EXPIRE_MINUTES", "2")
	if Load().UpstreamTimeout != time.Second || Load().JWTExpire != 2*time.Minute {
		t.Fatal("wrong duration unit")
	}
	t.Setenv("UPSTREAM_TIMEOUT_SECONDS", "0")
	if Load().UpstreamTimeout != 180*time.Second {
		t.Fatal("zero must not disable upstream timeout")
	}
	t.Setenv("RATE_LIMIT_FAIL_OPEN", "false")
	if Load().RateLimitFailOpen {
		t.Fatal("fail closed expected")
	}
	t.Setenv("RATE_LIMIT_FAIL_OPEN", "true")
	if !Load().RateLimitFailOpen {
		t.Fatal("fail-open configuration ignored")
	}
}

func TestPrivacyCollectionIsOptIn(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("PROMPT_CAPTURE_ENABLED", "")
	t.Setenv("SEED_DATA", "")
	cfg := Load()
	if cfg.CapturePrompts || cfg.SeedData {
		t.Fatal("prompt capture and seed data must be disabled by default")
	}
	if cfg.PromptRetentionDays != 30 || cfg.MaxRequestBodyBytes <= 0 {
		t.Fatalf("unexpected safe defaults: retention=%d body_limit=%d", cfg.PromptRetentionDays, cfg.MaxRequestBodyBytes)
	}
}

func TestProductionConfigurationRequiresExplicitSecretsAndSafeModes(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("MYSQL_DSN", "root:root@tcp(mysql:3306)/aegis")
	t.Setenv("AUTO_MIGRATE", "")
	t.Setenv("SEED_DATA", "")
	t.Setenv("RATE_LIMIT_FAIL_OPEN", "")
	cfg := Load()
	if cfg.JWTSecret != "" || cfg.AutoMigrate || cfg.SeedData {
		t.Fatalf("unsafe production defaults: %+v", cfg)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production validation to reject missing secrets")
	}

	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
	t.Setenv("REDIS_PASSWORD", "redis-test-password")
	t.Setenv("MYSQL_DSN", "aegis_app:secret@tcp(mysql:3306)/aegis")
	cfg = Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("safe production config rejected: %v", err)
	}

	t.Setenv("AUTO_MIGRATE", "true")
	if err := Load().Validate(); err == nil {
		t.Fatal("production must reject GORM AutoMigrate")
	}
}

func TestProductionSeedRequiresUniqueAdminCredentials(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
	t.Setenv("REDIS_PASSWORD", "redis-test-password")
	t.Setenv("MYSQL_DSN", "aegis_app:secret@tcp(mysql:3306)/aegis")
	t.Setenv("AUTO_MIGRATE", "false")
	t.Setenv("SEED_DATA", "true")
	t.Setenv("BOOTSTRAP_ADMIN_USERNAME", "")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("seed mode must require explicit bootstrap credentials")
	}
	t.Setenv("BOOTSTRAP_ADMIN_USERNAME", "owner")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", strings.Repeat("p", 16))
	if err := Load().Validate(); err != nil {
		t.Fatalf("explicit bootstrap credentials rejected: %v", err)
	}
}

func TestProductionAliasNeverUsesDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("MYSQL_DSN", "root:root@tcp(mysql:3306)/aegis")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("AUTO_MIGRATE", "")
	cfg := Load()
	if cfg.JWTSecret != "" || cfg.AutoMigrate || !cfg.IsProduction() {
		t.Fatalf("production alias used unsafe defaults: %+v", cfg)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("production alias must enforce production validation")
	}
}

func TestDevelopmentRequiresExplicitDatabaseDSN(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("MYSQL_DSN", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("development must not silently fall back to a public database credential")
	}
}

func TestMigrationConfigNeedsOnlyDedicatedMySQLCredentials(t *testing.T) {
	cfg := &Config{Env: "production", MySQLDSN: "aegis_app:db-pass@tcp(mysql:3306)/aegis"}
	if err := cfg.ValidateMigration(); err != nil {
		t.Fatalf("MySQL-only migration config rejected: %v", err)
	}
	cfg.MySQLDSN = "root:root@tcp(mysql:3306)/aegis"
	if err := cfg.ValidateMigration(); err == nil {
		t.Fatal("production migration must reject root MySQL credentials")
	}
}
