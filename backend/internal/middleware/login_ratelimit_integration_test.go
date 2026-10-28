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

func loginTestHandler(t *testing.T, client *redis.Client) http.Handler {
	t.Helper()
	r := gin.New()
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
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"x"}`, username)))
	req.Header.Set("Content-Type", "application/json")
	if len(forwardedFor) > 0 {
		req.Header.Set("X-Forwarded-For", forwardedFor[0])
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
	client := redis.NewClient(&redis.Options{Addr: addr})
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

	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	handler := loginTestHandler(t, client)
	for i := 0; i < loginUsernameLimit; i++ {
		ip := fmt.Sprintf("198.51.100.%d:4321", i+1)
		if got := loginRequestFromIP(t, handler, " GLOBAL ", ip); got != http.StatusOK {
			t.Fatalf("global username attempt %d: got %d", i+1, got)
		}
	}
	if got := loginRequestFromIP(t, handler, "global", "203.0.113.1:4321"); got != http.StatusTooManyRequests {
		t.Fatalf("global username limit: got %d", got)
	}
}

func TestLoginRateLimitIgnoresUntrustedForwardedIP(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
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

func TestLoginRateLimitSharedAcrossInstancesAndHashedKeys(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
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
		if strings.Contains(key, "shared") || strings.Contains(key, "127.0.0.1") {
			t.Fatalf("plaintext identity in key %q", key)
		}
	}
}
