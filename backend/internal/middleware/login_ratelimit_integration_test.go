//go:build integration

package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"aegis-gateway/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Keep this package's FLUSHDB-based login tests away from other CI packages.
const loginRateLimitTestRedisDB = 1

func loginTestHandler(t *testing.T, client *redis.Client) http.Handler {
	t.Helper()
	r := gin.New()
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	r.POST("/login", LoginRateLimit(&repository.Repository{Redis: client}, time.Second), func(c *gin.Context) {
		var payload struct {
			Username string `json:"username"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.String(http.StatusOK, payload.Username)
	})
	return r
}

func loginTestServer(t *testing.T, client *redis.Client) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(loginTestHandler(t, client))
	t.Cleanup(server.Close)
	return server
}

func loginRequest(t *testing.T, serverURL, username string) int {
	t.Helper()
	resp, err := http.Post(serverURL+"/login", "application/json", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"x"}`, username)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func loginRequestFromIP(t *testing.T, handler http.Handler, username, remoteAddr string, forwardedFor ...string) int {
	t.Helper()
	headers := make(map[string]string)
	if len(forwardedFor) > 0 {
		headers["X-Forwarded-For"] = forwardedFor[0]
	}
	return loginRequestWithHeaders(t, handler, username, remoteAddr, headers)
}

func loginRequestWithHeaders(t *testing.T, handler http.Handler, username, remoteAddr string, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"x"}`, username)))
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	req.RemoteAddr = remoteAddr
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp.Code
}

func TestLoginRateLimitDimensionsAndBodyRestore(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: loginRateLimitTestRedisDB})
	defer client.Close()
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	server := loginTestServer(t, client)

	for i := 0; i < loginUserIPLimit; i++ {
		if got := loginRequest(t, server.URL, " Admin "); got != http.StatusOK {
			t.Fatalf("IP+username attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequest(t, server.URL, "admin"); got != http.StatusTooManyRequests {
		t.Fatalf("IP+username limit: got %d", got)
	}

	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < loginIPLimit; i++ {
		if got := loginRequest(t, server.URL, fmt.Sprintf("user-%d", i)); got != http.StatusOK {
			t.Fatalf("IP attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequest(t, server.URL, "user-final"); got != http.StatusTooManyRequests {
		t.Fatalf("IP limit: got %d", got)
	}
}

func TestLoginRateLimitDoesNotGloballyLockOutUsername(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: loginRateLimitTestRedisDB})
	defer client.Close()
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	handler := loginTestHandler(t, client)
	for i := 0; i < 35; i++ {
		ip := fmt.Sprintf("198.51.100.%d:4321", i+1)
		if got := loginRequestFromIP(t, handler, " targeted-user ", ip); got != http.StatusOK {
			t.Fatalf("distributed username attempt %d: got %d", i+1, got)
		}
	}

	// The same IP is still bounded by the per-IP+username bucket.
	for i := 0; i < loginUserIPLimit; i++ {
		if got := loginRequestFromIP(t, handler, "targeted-user", "203.0.113.1:4321"); got != http.StatusOK {
			t.Fatalf("per-IP username attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequestFromIP(t, handler, "targeted-user", "203.0.113.1:4321"); got != http.StatusTooManyRequests {
		t.Fatalf("per-IP username limit: got %d", got)
	}
}

func TestLoginRateLimitIgnoresForwardedIPFromUntrustedPeer(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: loginRateLimitTestRedisDB})
	defer client.Close()
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	handler := loginTestHandler(t, client)
	for i := 0; i < loginUserIPLimit; i++ {
		forwarded := fmt.Sprintf("198.51.100.%d", i+1)
		if got := loginRequestFromIP(t, handler, "proxy-shared", "203.0.113.9:4321", forwarded); got != http.StatusOK {
			t.Fatalf("forwarded header attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequestFromIP(t, handler, "proxy-shared", "203.0.113.9:4321", "198.51.100.200"); got != http.StatusTooManyRequests {
		t.Fatalf("spoofed forwarding headers bypassed source-IP limit: got %d", got)
	}
}

func TestLoginRateLimitSeparatesClientsBehindTrustedProxy(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: loginRateLimitTestRedisDB})
	defer client.Close()
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := r.SetTrustedProxies([]string{"198.51.100.0/24"}); err != nil {
		t.Fatal(err)
	}
	r.POST("/login", LoginRateLimit(&repository.Repository{Redis: client}, time.Second), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handler := http.Handler(r)

	for i := 0; i < loginUserIPLimit; i++ {
		if got := loginRequestFromIP(t, handler, "shared-account", "198.51.100.10:4321", "203.0.113.1"); got != http.StatusOK {
			t.Fatalf("first client attempt %d: got %d", i+1, got)
		}
		if got := loginRequestFromIP(t, handler, "shared-account", "198.51.100.10:4321", "203.0.113.2"); got != http.StatusOK {
			t.Fatalf("second client attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequestFromIP(t, handler, "shared-account", "198.51.100.10:4321", "203.0.113.1"); got != http.StatusTooManyRequests {
		t.Fatalf("first client should hit its own IP+username limit: got %d", got)
	}
	if got := loginRequestFromIP(t, handler, "shared-account", "198.51.100.10:4321", "203.0.113.2"); got != http.StatusTooManyRequests {
		t.Fatalf("second client should hit its own IP+username limit: got %d", got)
	}
}

func TestLoginRateLimitIgnoresXRealIPEvenFromTrustedProxy(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: loginRateLimitTestRedisDB})
	defer client.Close()
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := r.SetTrustedProxies([]string{"198.51.100.0/24"}); err != nil {
		t.Fatal(err)
	}
	r.POST("/login", LoginRateLimit(&repository.Repository{Redis: client}, time.Second), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handler := http.Handler(r)

	for i := 0; i < loginUserIPLimit; i++ {
		realIP := fmt.Sprintf("203.0.113.%d", i+1)
		if got := loginRequestWithHeaders(t, handler, "shared-account", "198.51.100.10:4321", map[string]string{"X-Real-IP": realIP}); got != http.StatusOK {
			t.Fatalf("attempt %d with X-Real-IP %s: got %d", i+1, realIP, got)
		}
	}
	if got := loginRequestWithHeaders(t, handler, "shared-account", "198.51.100.10:4321", map[string]string{"X-Real-IP": "203.0.113.200"}); got != http.StatusTooManyRequests {
		t.Fatalf("X-Real-IP must not split the trusted proxy's source bucket: got %d", got)
	}
}

func TestLoginRateLimitSharedAcrossInstancesAndHashedKeys(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: loginRateLimitTestRedisDB})
	defer client.Close()
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	first, second := loginTestServer(t, client), loginTestServer(t, client)
	for i := 0; i < loginUserIPLimit; i++ {
		server := first
		if i%2 == 1 {
			server = second
		}
		if got := loginRequest(t, server.URL, "shared"); got != http.StatusOK {
			t.Fatalf("shared instance attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequest(t, second.URL, "shared"); got != http.StatusTooManyRequests {
		t.Fatalf("shared instance limit: got %d", got)
	}

	keys, err := client.Keys(t.Context(), "auth:login:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "auth:login:user:") {
			t.Fatalf("unexpected global username lockout key %q", key)
		}
		if strings.Contains(key, "shared") || strings.Contains(key, "127.0.0.1") {
			t.Fatalf("plaintext identity in key %q", key)
		}
	}
}
