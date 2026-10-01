//go:build integration

package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func limiterServer(t *testing.T, addr string, id uint64, failOpen bool) *httptest.Server {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: addr, ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close() })
	cfg := config.Load()
	cfg.RateLimitFailOpen = failOpen
	cfg.RateLimitTimeout = 100 * time.Millisecond
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxAPIKey, &model.ApiKey{ID: id, RPSLimit: 1, Burst: 1}); c.Next() }, RateLimit(&repository.Repository{Redis: client}, cfg))
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	s := httptest.NewServer(r)
	t.Cleanup(s.Close)
	return s
}
func TestRedisTwoInstanceBucket(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	id := uint64(time.Now().UnixNano())
	servers := []*httptest.Server{limiterServer(t, addr, id, false), limiterServer(t, addr, id, false)}
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(s *httptest.Server) {
			defer wg.Done()
			<-start
			resp, err := http.Get(s.URL)
			if err != nil {
				results <- 0
				return
			}
			resp.Body.Close()
			results <- resp.StatusCode
		}(s)
	}
	close(start)
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	if counts[200] != 1 || counts[429] != 1 {
		t.Fatalf("shared token bucket: %v", counts)
	}
}
func TestRedisFaultPolicyDeadline(t *testing.T) {
	// Accept connections without replying: verifies actual socket deadlines,
	// not merely a context cancellation against an immediately refused port.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	defer func() {
		_ = l.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for _, open := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail_open_%t", open), func(t *testing.T) {
			for i := 0; i < 2; i++ {
				s := limiterServer(t, l.Addr().String(), uint64(i+1), open)
				started := time.Now()
				resp, err := (&http.Client{Timeout: time.Second}).Get(s.URL)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				want := 503
				if open {
					want = 200
				}
				if resp.StatusCode != want {
					t.Fatalf("got %d want %d", resp.StatusCode, want)
				}
				if time.Since(started) > time.Second {
					t.Fatal("dependency deadline exceeded")
				}
			}
		})
	}
}
