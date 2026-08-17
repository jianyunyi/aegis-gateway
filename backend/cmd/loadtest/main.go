// 压测工具：对网关做并发请求，输出 RPS 与延迟分位数。
// 用法：go run ./cmd/loadtest -url http://localhost:8081/healthz -concurrency 500 -requests 2000
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	url         = flag.String("url", "http://localhost:8081/healthz", "压测 URL")
	concurrency = flag.Int("concurrency", 500, "并发数")
	requests    = flag.Int("requests", 2000, "总请求数")
	method      = flag.String("method", "GET", "HTTP 方法")
	body        = flag.String("body", "", "请求体（POST 时用）")
	key         = flag.String("key", "", "Bearer API Key（可选）")
	timeout     = flag.Duration("timeout", 30*time.Second, "单请求超时")
)

func main() {
	flag.Parse()

	var (
		mu        sync.Mutex
		latencies []int64
		errors    atomic.Int64
		sent      atomic.Int64
		wg        sync.WaitGroup
	)
	client := &http.Client{Timeout: *timeout}

	worker := func() {
		defer wg.Done()
		var reqBody io.Reader
		if *body != "" {
			reqBody = bytes.NewReader([]byte(*body))
		}
		for {
			n := sent.Add(1)
			if n > int64(*requests) {
				return
			}
			start := time.Now()
			req, _ := http.NewRequest(*method, *url, reqBody)
			if *key != "" {
				req.Header.Set("Authorization", "Bearer "+*key)
			}
			if *body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode >= 500 {
				errors.Add(1)
				if resp != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
				continue
			}
			if resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			lat := time.Since(start).Milliseconds()
			mu.Lock()
			latencies = append(latencies, lat)
			mu.Unlock()
		}
	}

	ctx := context.Background()
	_ = ctx
	startAll := time.Now()
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go worker()
	}
	wg.Wait()
	total := time.Since(startAll)

	mu.Lock()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	mu.Unlock()

	success := int64(len(latencies))
	fmt.Printf("目标: %s (并发 %d, 请求 %d)\n", *url, *concurrency, *requests)
	fmt.Printf("耗时: %v | RPS: %.1f | 成功: %d | 失败: %d\n",
		total.Round(time.Millisecond), float64(success)/total.Seconds(), success, errors.Load())
	if success > 0 {
		pct := func(p float64) int64 {
			idx := int(float64(len(latencies)-1) * p)
			return latencies[idx]
		}
		fmt.Printf("延迟分位数(ms): p50=%d p90=%d p95=%d p99=%d max=%d\n",
			pct(0.50), pct(0.90), pct(0.95), pct(0.99), latencies[len(latencies)-1])
	}
}
