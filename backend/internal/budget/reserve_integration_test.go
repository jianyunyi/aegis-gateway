//go:build integration

package budget

import (
	"context"
	"fmt"
	"os"
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
}
