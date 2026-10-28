//go:build integration

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestIdempotencyClaimHeartbeatAndCompletion(t *testing.T) {
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
	repo := &Repository{Redis: client}
	keyID, key := uint64(time.Now().UnixNano()), "integration-idempotency"
	fp := IdempotencyFingerprint("POST", "/v1/test", "application/json", []byte(`{"body":"one"}`))
	state, owner, err := repo.ClaimIdempotency(ctx, keyID, key, fp)
	if err != nil || !owner || state.State != "processing" {
		t.Fatalf("claim owner=%v state=%+v err=%v", owner, state, err)
	}
	other := IdempotencyFingerprint("POST", "/v1/test", "application/json", []byte(`{"body":"two"}`))
	state, owner, err = repo.ClaimIdempotency(ctx, keyID, key, other)
	if err != nil || owner || state.Fingerprint != fp {
		t.Fatalf("different body overwrote claim: owner=%v state=%+v err=%v", owner, state, err)
	}
	time.Sleep(25 * time.Millisecond)
	before, _ := client.PTTL(ctx, idempotencyRedisKey(keyID, key)).Result()
	ok, err := repo.RenewIdempotency(ctx, keyID, key, fp)
	if err != nil || !ok {
		t.Fatalf("heartbeat ok=%v err=%v", ok, err)
	}
	after, _ := client.PTTL(ctx, idempotencyRedisKey(keyID, key)).Result()
	if after <= before {
		t.Fatalf("heartbeat did not extend TTL: before=%v after=%v", before, after)
	}
	if err := repo.CompleteIdempotency(ctx, keyID, key, IdempotencyResult{Fingerprint: fp, Status: 200, Body: []byte(`ok`)}); err != nil {
		t.Fatal(err)
	}
	ok, err = repo.RenewIdempotency(ctx, keyID, key, fp)
	if err != nil || ok {
		t.Fatalf("completed claim was renewed: ok=%v err=%v", ok, err)
	}

	unreplayableKey := "integration-unreplayable"
	unreplayableFP := IdempotencyFingerprint("POST", "/v1/large", "application/json", []byte(`{"large":true}`))
	if _, owner, err := repo.ClaimIdempotency(ctx, keyID, unreplayableKey, unreplayableFP); err != nil || !owner {
		t.Fatalf("unreplayable claim owner=%v err=%v", owner, err)
	}
	if err := repo.MarkIdempotencyUnreplayable(ctx, keyID, unreplayableKey, unreplayableFP); err != nil {
		t.Fatal(err)
	}
	state, owner, err = repo.ClaimIdempotency(ctx, keyID, unreplayableKey, unreplayableFP)
	if err != nil || owner || state.State != "unreplayable" {
		t.Fatalf("unreplayable retry owner=%v state=%+v err=%v", owner, state, err)
	}
}
