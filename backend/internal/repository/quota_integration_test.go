//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestAtomicQuotaReservationsDoNotOversell(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{Redis: client}
	keyID := uint64(time.Now().UnixNano())
	const requests = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	var admitted []string
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ok, err := repo.ReserveQuota(ctx, keyID, fmt.Sprintf("req-%d", i), 0, 10, 1, time.Minute)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if ok {
				mu.Lock()
				allowed++
				admitted = append(admitted, fmt.Sprintf("req-%d", i))
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if allowed != 10 {
		t.Fatalf("allowed %d concurrent requests, want exactly 10", allowed)
	}
	for _, requestID := range admitted {
		if err := repo.ReleaseQuota(ctx, keyID, requestID); err != nil {
			t.Fatal(err)
		}
	}

	id := "retry-id"
	first, err := repo.ReserveQuota(ctx, keyID, id, 0, 10, 3, time.Minute)
	if err != nil || !first {
		t.Fatalf("first reservation allowed=%v err=%v", first, err)
	}
	retry, err := repo.ReserveQuota(ctx, keyID, id, 0, 10, 3, time.Minute)
	if err != nil || !retry {
		t.Fatalf("same-ID retry allowed=%v err=%v", retry, err)
	}
	if err := repo.ReleaseQuota(ctx, keyID, id); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReleaseQuota(ctx, keyID, id); err != nil {
		t.Fatal(err)
	}

	expiredID := "abandoned"
	expired, err := repo.ReserveQuota(ctx, keyID, expiredID, 0, 1, 1, 25*time.Millisecond)
	if err != nil || !expired {
		t.Fatalf("short reservation allowed=%v err=%v", expired, err)
	}
	live, err := repo.ReserveQuota(ctx, keyID, "live", 0, 2, 1, time.Minute)
	if err != nil || !live {
		t.Fatalf("live reservation allowed=%v err=%v", live, err)
	}
	time.Sleep(60 * time.Millisecond)
	reclaimed, err := repo.ReserveQuota(ctx, keyID, "after-expiry", 0, 2, 1, time.Minute)
	if err != nil || !reclaimed {
		t.Fatalf("expired reservation was not reclaimed: allowed=%v err=%v", reclaimed, err)
	}
	reserved, err := client.Get(ctx, fmt.Sprintf("quota:reserved:%d", keyID)).Int64()
	if err != nil || reserved != 2 {
		t.Fatalf("reserved after expiry=%d error=%v", reserved, err)
	}
	if err := repo.ReleaseQuota(ctx, keyID, "live"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReleaseQuota(ctx, keyID, "after-expiry"); err != nil {
		t.Fatal(err)
	}
}
