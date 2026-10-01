package config

import (
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
		t.Fatal("fail open configuration ignored")
	}
}
