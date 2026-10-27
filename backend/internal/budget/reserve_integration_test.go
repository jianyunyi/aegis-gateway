//go:build integration

package budget

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"aegis-gateway/internal/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMonthlyBudgetReservationsIncludeConcurrentRequests(t *testing.T) {
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
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UsageLog{}); err != nil {
		t.Fatal(err)
	}
	keyID := uint64(time.Now().UnixNano())
	if err := db.Create(&model.UsageLog{RequestID: fmt.Sprintf("base-%d", keyID), APIKeyID: keyID, ModelName: "m", Kind: "chat", Cost: 1, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	b := New(client, db)
	const requests = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	var admitted []string
	for i := 0; i < requests; i++ {
		requestID := fmt.Sprintf("budget-%d", i)
		wg.Add(1)
		go func(requestID string) {
			defer wg.Done()
			<-start
			ok, err := b.Reserve(ctx, keyID, requestID, 2, 0.25, time.Minute)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if ok {
				mu.Lock()
				allowed++
				admitted = append(admitted, requestID)
				mu.Unlock()
			}
		}(requestID)
	}
	close(start)
	wg.Wait()
	if allowed != 4 {
		t.Fatalf("allowed %d concurrent requests, want exactly 4 after 1.0 committed against 2.0 limit", allowed)
	}
	monthKey := MonthKey(keyID)
	committedID := admitted[0]
	if err := db.Create(&model.UsageLog{RequestID: "settled-cost", APIKeyID: keyID, ModelName: "m", Kind: "chat", Cost: 0.25, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := b.SettleForMonth(ctx, keyID, monthKey, committedID, 0.25); err != nil {
		t.Fatal(err)
	}
	for _, requestID := range admitted[1:] {
		if err := b.ReleaseForMonth(ctx, keyID, monthKey, requestID); err != nil {
			t.Fatal(err)
		}
	}
	atLimit, err := b.ReserveForMonth(ctx, keyID, monthKey, "at-limit", 2, 0.75, time.Minute)
	if err != nil || !atLimit {
		t.Fatalf("exact remaining budget allowed=%v err=%v", atLimit, err)
	}
	overLimit, err := b.ReserveForMonth(ctx, keyID, monthKey, "over-limit", 2, 0.01, time.Minute)
	if err != nil || overLimit {
		t.Fatalf("in-flight reservation oversold budget: allowed=%v err=%v", overLimit, err)
	}
	if err := b.ReleaseForMonth(ctx, keyID, monthKey, "at-limit"); err != nil {
		t.Fatal(err)
	}
	expired, err := b.ReserveForMonth(ctx, keyID, monthKey, "abandoned", 2, 0.25, 25*time.Millisecond)
	if err != nil || !expired {
		t.Fatalf("short reservation allowed=%v err=%v", expired, err)
	}
	live, err := b.ReserveForMonth(ctx, keyID, monthKey, "live", 2, 0.5, time.Minute)
	if err != nil || !live {
		t.Fatalf("live reservation allowed=%v err=%v", live, err)
	}
	time.Sleep(60 * time.Millisecond)
	reclaimed, err := b.ReserveForMonth(ctx, keyID, monthKey, "after-expiry", 2, 0.25, time.Minute)
	if err != nil || !reclaimed {
		t.Fatalf("expired budget reservation was not reclaimed: allowed=%v err=%v", reclaimed, err)
	}
	reserved, err := client.Get(ctx, "budget:reserved:"+strings.TrimPrefix(monthKey, "budget:")).Int64()
	if err != nil || reserved != 750_000 {
		t.Fatalf("reserved after expiry=%d error=%v", reserved, err)
	}
	if err := b.ReleaseForMonth(ctx, keyID, monthKey, "after-expiry"); err != nil {
		t.Fatal(err)
	}
	if err := b.ReleaseForMonth(ctx, keyID, monthKey, "live"); err != nil {
		t.Fatal(err)
	}
}

func TestMonthlyBudgetCommitAndReservationDoNotDoubleCount(t *testing.T) {
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
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UsageLog{}); err != nil {
		t.Fatal(err)
	}
	keyID := uint64(time.Now().UnixNano())
	monthKey := MonthKey(keyID)
	if err := db.Create(&model.UsageLog{RequestID: fmt.Sprintf("base-%d", keyID), APIKeyID: keyID, ModelName: "m", Kind: "chat", Cost: 1, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	b := New(client, db)
	firstID := fmt.Sprintf("first-%d", keyID)
	allowed, err := b.ReserveForMonth(ctx, keyID, monthKey, firstID, 2, 0.25, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("first reservation allowed=%v err=%v", allowed, err)
	}
	inserted := make(chan struct{})
	releaseCommit := make(chan struct{})
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- b.CommitUsageForMonth(ctx, keyID, monthKey, firstID, 0.25, func() error {
			if err := db.Create(&model.UsageLog{RequestID: firstID, APIKeyID: keyID, ModelName: "m", Kind: "chat", Cost: 0.25, CreatedAt: time.Now()}).Error; err != nil {
				return err
			}
			close(inserted)
			<-releaseCommit
			return nil
		})
	}()
	<-inserted
	type reserveResult struct {
		allowed bool
		err     error
	}
	reserveDone := make(chan reserveResult, 1)
	go func() {
		ok, err := b.ReserveForMonth(ctx, keyID, monthKey, "second", 2, 0.75, time.Minute)
		reserveDone <- reserveResult{allowed: ok, err: err}
	}()
	select {
	case result := <-reserveDone:
		close(releaseCommit)
		<-commitDone
		t.Fatalf("reservation was not serialized with settlement: allowed=%v err=%v", result.allowed, result.err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCommit)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	result := <-reserveDone
	if result.err != nil || !result.allowed {
		t.Fatalf("request at remaining budget was rejected: allowed=%v err=%v", result.allowed, result.err)
	}
	used, err := client.Get(ctx, monthKey).Int64()
	if err != nil || used != 1_250_000 {
		t.Fatalf("committed projection=%d error=%v", used, err)
	}
	reserved, err := client.Get(ctx, "budget:reserved:"+strings.TrimPrefix(monthKey, "budget:")).Int64()
	if err != nil || reserved != 750_000 {
		t.Fatalf("in-flight reservation=%d error=%v", reserved, err)
	}
	if err := b.ReleaseForMonth(ctx, keyID, monthKey, "second"); err != nil {
		t.Fatal(err)
	}
}
