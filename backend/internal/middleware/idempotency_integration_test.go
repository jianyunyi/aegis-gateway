//go:build integration

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

func TestIdempotencyMiddlewareDoesNotRunHandlerTwice(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	repo := &repository.Repository{Redis: client}
	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	apiKey := &model.ApiKey{ID: uint64(time.Now().UnixNano())}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxAPIKey, apiKey); c.Next() })
	r.Use(Idempotency(repo))
	r.POST("/test", func(c *gin.Context) {
		calls.Add(1)
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})
	r.POST("/other", func(c *gin.Context) {
		calls.Add(1)
		c.JSON(http.StatusCreated, gin.H{"other": true})
	})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"x":1}`))
		req.Header.Set(IdempotencyKeyHeader, "same-key")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("request %d status=%d body=%s", i, w.Code, w.Body.String())
		}
		if i == 0 {
			fingerprint := repository.IdempotencyFingerprint(http.MethodPost, "/test", "", []byte(`{"x":1}`))
			state, owner, err := repo.ClaimIdempotency(ctx, apiKey.ID, "same-key", fingerprint)
			if err != nil || owner || state.State != "done" {
				t.Fatalf("first request did not store replay state: owner=%v state=%+v err=%v", owner, state, err)
			}
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls=%d, want 1", calls.Load())
	}
	conflict := httptest.NewRequest(http.MethodPost, "/other", strings.NewReader(`{"x":1}`))
	conflict.Header.Set(IdempotencyKeyHeader, "same-key")
	conflictWriter := httptest.NewRecorder()
	r.ServeHTTP(conflictWriter, conflict)
	if conflictWriter.Code != http.StatusConflict {
		t.Fatalf("different endpoint status=%d body=%s, want %d", conflictWriter.Code, conflictWriter.Body.String(), http.StatusConflict)
	}
	if calls.Load() != 1 {
		t.Fatalf("conflicting endpoint reached handler: calls=%d, want 1", calls.Load())
	}
}
