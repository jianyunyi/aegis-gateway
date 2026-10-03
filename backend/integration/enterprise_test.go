//go:build integration

// These tests start two independent HTTP gateway instances, sharing real MySQL
// and Redis. TCP fault gates drop pooled connections as well as new connections.
package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"aegis-gateway/internal/config"
	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/router"
	"aegis-gateway/internal/service"
)

type faultGate struct {
	listener net.Listener
	target   string
	mu       sync.Mutex
	down     bool
	stall    bool
	conns    map[net.Conn]bool
}

// Stall quota projection only, so the second instance must commit its ledger
// while the first instance is waiting for Redis.
type quotaStallHook struct {
	entered      chan struct{}
	release      chan struct{}
	rateFailures atomic.Int32
}

func (h *quotaStallHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *quotaStallHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *quotaStallHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "set" || cmd.Name() == "eval" || cmd.Name() == "evalsha" {
			for _, arg := range cmd.Args() {
				if key, ok := arg.(string); ok && strings.HasPrefix(key, "rl:{") {
					h.rateFailures.Add(1)
					return fmt.Errorf("injected Redis rate-limit failure")
				}
				if key, ok := arg.(string); ok && strings.HasPrefix(key, "quota:") {
					h.entered <- struct{}{}
					select {
					case <-h.release:
						return next(ctx, cmd)
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
		}
		return next(ctx, cmd)
	}
}

func TestTwoInstancesFailOpenUsageProjection(t *testing.T) {
	f := setup(t, true)
	h := &quotaStallHook{entered: make(chan struct{}, 8), release: make(chan struct{})}
	for _, repo := range f.repos {
		repo.Redis.AddHook(h)
	}
	defer close(h.release)
	errs := make(chan error, 2)
	request := func(i int) {
		resp, err := f.chat(i, fmt.Sprintf("projection-%d", i), false)
		if err == nil {
			_, err = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				err = fmt.Errorf("status %d", resp.StatusCode)
			}
		}
		errs <- err
	}
	go request(0)
	select {
	case <-h.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first projection did not start")
	}
	go request(1)
	deadline := time.Now().Add(750 * time.Millisecond)
	for {
		var key model.ApiKey
		var rows int64
		if err := f.repos[0].DB.First(&key, f.key.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.repos[0].DB.Model(&model.UsageLog{}).Where("api_key_id = ?", key.ID).Count(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if rows == 2 && key.UsedTokens == 60 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Redis stalled concurrent ledger: rows=%d tokens=%d", rows, key.UsedTokens)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if h.rateFailures.Load() != 2 {
		t.Fatalf("fail-open not exercised: %d limiter failures", h.rateFailures.Load())
	}
}

func TestQuotaProjectionDoesNotRegress(t *testing.T) {
	f := setup(t, false)
	ctx := context.Background()
	key := fmt.Sprintf("quota:%d", f.key.ID)
	for _, total := range []int64{60, 30, 9007199254740993, 9007199254740992} {
		if err := f.repos[0].ProjectQuota(ctx, f.key.ID, total); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.repos[1].Redis.Get(ctx, key).Int64()
	if err != nil || got != 9007199254740993 {
		t.Fatalf("projection=%d error=%v", got, err)
	}
	if _, _, err := service.NewBillingService(f.repos[1]).ReconcileQuota(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = f.repos[0].Redis.Get(ctx, key).Int64()
	if err != nil || got != 0 {
		t.Fatalf("reconciled=%d error=%v", got, err)
	}
}

func newGate(t *testing.T, target string) *faultGate {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &faultGate{listener: l, target: target, conns: map[net.Conn]bool{}}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go g.forward(c)
		}
	}()
	t.Cleanup(func() { g.setDown(true); _ = l.Close() })
	return g
}
func (g *faultGate) forward(c net.Conn) {
	g.mu.Lock()
	if g.down {
		g.mu.Unlock()
		_ = c.Close()
		return
	}
	g.conns[c] = true
	if g.stall {
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()
	upstream, err := net.DialTimeout("tcp", g.target, time.Second)
	if err != nil {
		_ = c.Close()
		g.mu.Lock()
		delete(g.conns, c)
		g.mu.Unlock()
		return
	}
	g.mu.Lock()
	if g.down {
		g.mu.Unlock()
		_ = c.Close()
		_ = upstream.Close()
		return
	}
	g.conns[upstream] = true
	g.mu.Unlock()
	defer func() {
		_ = c.Close()
		_ = upstream.Close()
		g.mu.Lock()
		delete(g.conns, c)
		delete(g.conns, upstream)
		g.mu.Unlock()
	}()
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, c); _ = upstream.Close(); close(done) }()
	_, _ = io.Copy(c, upstream)
	_ = c.Close()
	<-done
}
func (g *faultGate) setDown(down bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.down = down
	if down {
		for c := range g.conns {
			_ = c.Close()
		}
	}
}

func (g *faultGate) setStall(stall bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stall = stall
	for c := range g.conns {
		_ = c.Close()
		delete(g.conns, c)
	}
}

type fixture struct {
	cfg                  *config.Config
	repos                []*repository.Repository
	servers              []*httptest.Server
	redisGate, mysqlGate *faultGate
	token                string
	key                  *model.ApiKey
}

func setup(t *testing.T, failOpen bool) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	addr := os.Getenv("TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Fatal("integration requires TEST_MYSQL_DSN and TEST_REDIS_ADDR")
	}
	// CI supplies a disposable database; never run these tests against production.
	hostStart := strings.Index(dsn, "@tcp(")
	if hostStart < 0 {
		t.Fatal("TCP DSN required")
	}
	hostStart += 5
	hostEnd := strings.Index(dsn[hostStart:], ")")
	if hostEnd < 0 {
		t.Fatal("invalid DSN")
	}
	hostEnd += hostStart
	f := &fixture{redisGate: newGate(t, addr), mysqlGate: newGate(t, dsn[hostStart:hostEnd])}
	cfg := config.Load()
	cfg.MySQLDSN = dsn[:hostStart] + f.mysqlGate.listener.Addr().String() + dsn[hostEnd:]
	cfg.RedisAddr = f.redisGate.listener.Addr().String()
	cfg.SeedData = false
	cfg.AutoMigrate = false
	cfg.RateLimitFailOpen = failOpen
	cfg.RateLimitTimeout = 200 * time.Millisecond
	cfg.ReadinessTimeout = 300 * time.Millisecond
	cfg.UpstreamTimeout = time.Second
	cfg.CacheTTL = time.Second
	f.cfg = cfg
	for i := 0; i < 2; i++ {
		repo, err := repository.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		f.repos = append(f.repos, repo)
		t.Cleanup(repo.Close)
	}
	if err := f.repos[0].AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "slow") {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
				return
			}
		}
		if strings.Contains(string(raw), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": heartbeat\n\ndata: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
			w.(http.Flusher).Flush()
			if strings.Contains(string(raw), "cancel") {
				<-r.Context().Done()
				return
			}
			if strings.Contains(string(raw), "truncate") {
				return
			}
			done := "data: [DONE]\n\n"
			if strings.Contains(string(raw), "compact_done") {
				done = "data:[DONE]\n\n"
			}
			if strings.Contains(string(raw), "compact_done_crlf") {
				done = "data:[DONE]\r\n\r\n"
			}
			fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20,\"total_tokens\":30}}\n\n"+done)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	}))
	t.Cleanup(upstream.Close)
	name := fmt.Sprintf("test-%d", time.Now().UnixNano())
	p, err := service.NewProviderService(f.repos[0], cfg.JWTSecret).Create(name, upstream.URL, "test-key", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repos[0].DB.Create(&model.Model{Name: name, ProviderID: p.ID, Tier: "cheap", Enabled: 1, PriceIn: 1, PriceOut: 1}).Error; err != nil {
		t.Fatal(err)
	}
	f.key, f.token, err = service.NewKeyService(f.repos[0]).Create(1, name, 100000, 100000, 1000000, name, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range f.repos {
		srv := httptest.NewServer(router.New(cfg, repo))
		f.servers = append(f.servers, srv)
		t.Cleanup(srv.Close)
	}
	return f
}
func (f *fixture) chat(i int, prompt string, stream bool) (*http.Response, error) {
	req, _ := http.NewRequest("POST", f.servers[i].URL+"/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":%q}],"stream":%t}`, f.key.DefaultModel, prompt, stream)))
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	return (&http.Client{Timeout: 4 * time.Second}).Do(req)
}
func assertStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s status %d want %d", url, resp.StatusCode, want)
	}
}
func TestTwoInstancesSharedRateLimit(t *testing.T) {
	f := setup(t, false)
	if err := f.repos[0].DB.Model(f.key).Updates(map[string]any{"rps_limit": 1, "burst": 1}).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			resp, err := f.chat(i, "rate", false)
			if err != nil {
				results <- 0
				return
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			results <- resp.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for code := range results {
		counts[code]++
	}
	if counts[200] != 1 || counts[429] != 1 {
		t.Fatalf("shared bucket results: %v", counts)
	}
}
func TestTwoInstancesDependencyFaults(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail_open_%t", open), func(t *testing.T) {
			f := setup(t, open)
			for _, s := range f.servers {
				assertStatus(t, s.URL+"/readyz", 200)
			}
			f.redisGate.setDown(true)
			for i, s := range f.servers {
				assertStatus(t, s.URL+"/healthz", 200)
				assertStatus(t, s.URL+"/readyz", 503)
				req, _ := http.NewRequest("GET", f.servers[i].URL+"/v1/models", nil)
				req.Header.Set("Authorization", "Bearer "+f.token)
				resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
				if err != nil {
					t.Fatal(err)
				}
				want := 503
				if open {
					want = 200
				}
				if resp.StatusCode != want {
					t.Errorf("got %d want %d", resp.StatusCode, want)
				}
				resp.Body.Close()
			}
			f.redisGate.setDown(false)
			for _, s := range f.servers {
				assertStatus(t, s.URL+"/readyz", 200)
			}
			f.mysqlGate.setDown(true)
			for _, s := range f.servers {
				assertStatus(t, s.URL+"/readyz", 503)
				assertStatus(t, s.URL+"/healthz", 200)
			}
			f.mysqlGate.setDown(false)
			for i, s := range f.servers {
				assertStatus(t, s.URL+"/readyz", 200)
				f.repos[i].Draining.Store(true)
				assertStatus(t, s.URL+"/readyz", 503)
			}
		})
	}
}
func TestTwoInstancesRedisDeadline(t *testing.T) {
	f := setup(t, false)
	f.redisGate.setStall(true)
	for i, s := range f.servers {
		started := time.Now()
		assertStatus(t, s.URL+"/readyz", 503)
		if time.Since(started) > time.Second {
			t.Fatal("readiness exceeded bounded deadline")
		}
		req, _ := http.NewRequest("GET", f.servers[i].URL+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+f.token)
		started = time.Now()
		resp, err := (&http.Client{Timeout: time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 503 || time.Since(started) > time.Second {
			t.Fatal("rate limiter exceeded bounded deadline")
		}
	}
	f.redisGate.setStall(false)
	for _, s := range f.servers {
		assertStatus(t, s.URL+"/readyz", 200)
	}
}

func TestTwoInstancesUsageAndReconcile(t *testing.T) {
	f := setup(t, false)
	ctx := context.Background()
	errs := make(chan error, 22)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := f.chat(i%2, fmt.Sprintf("request %d", i), false)
			if err == nil {
				_, err = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					err = fmt.Errorf("status %d", resp.StatusCode)
				}
			}
			errs <- err
		}(i)
	}
	for _, repo := range f.repos {
		wg.Add(1)
		go func(repo *repository.Repository) {
			defer wg.Done()
			_, err := service.NewBillingService(repo).Reconcile(ctx, 7)
			errs <- err
		}(repo)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, repo := range f.repos {
		if _, err := service.NewBillingService(repo).Reconcile(ctx, 7); err != nil {
			t.Fatal(err)
		}
	}
	var key model.ApiKey
	if err := f.repos[0].DB.First(&key, f.key.ID).Error; err != nil {
		t.Fatal(err)
	}
	redisTotal, err := f.repos[0].Redis.Get(ctx, fmt.Sprintf("quota:%d", key.ID)).Int64()
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	if err := f.repos[0].DB.Model(&model.UsageLog{}).Where("api_key_id = ?", key.ID).Select("COALESCE(SUM(total_tokens),0)").Scan(&total).Error; err != nil {
		t.Fatal(err)
	}
	if key.UsedTokens != 600 || redisTotal != 600 || total != 600 {
		t.Fatalf("mysql=%d redis=%d ledger=%d", key.UsedTokens, redisTotal, total)
	}
	var rows []model.BillingDaily
	if err := f.repos[0].DB.Where("api_key_id = ?", key.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TotalTokens != 600 {
		t.Fatalf("billing: %+v", rows)
	}
}
func TestTwoInstancesStreamResults(t *testing.T) {
	f := setup(t, false)
	for i := 0; i < 2; i++ {
		for _, tc := range []struct {
			prompt string
			status int
			code   string
		}{{"normal", 0, ""}, {"compact_done", 0, ""}, {"compact_done_crlf", 0, ""}, {"truncate", 502, "upstream_stream_incomplete"}, {"cancel", 499, "client_disconnect"}, {"slow", 502, "upstream_error"}} {
			started := time.Now()
			resp, err := f.chat(i, tc.prompt, true)
			if err != nil {
				t.Fatal(err)
			}
			rid := resp.Header.Get("X-Request-ID")
			if tc.prompt == "cancel" {
				buf := make([]byte, 32)
				_, _ = resp.Body.Read(buf)
				resp.Body.Close()
			} else {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			if tc.prompt == "slow" && time.Since(started) > 1500*time.Millisecond {
				t.Fatal("upstream second-based timeout not enforced")
			}
			var row model.UsageLog
			deadline := time.Now().Add(3 * time.Second)
			for {
				err = f.repos[0].DB.Where("request_id = ?", rid).First(&row).Error
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if int(row.Status) != tc.status || row.ErrorCode != tc.code {
				t.Fatalf("%s: status=%d code=%s", tc.prompt, row.Status, row.ErrorCode)
			}
			if tc.prompt == "normal" && (row.TTFTMs == nil || *row.TTFTMs < 25) {
				t.Fatalf("TTFT includes heartbeat: %v", row.TTFTMs)
			}
		}
	}
	var successes int64
	if err := f.repos[0].DB.Model(&model.UsageLog{}).Where("api_key_id = ? AND status = 0", f.key.ID).Count(&successes).Error; err != nil {
		t.Fatal(err)
	}
	if successes != 6 {
		t.Fatalf("success count %d", successes)
	}
}
