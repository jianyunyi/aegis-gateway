package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	idempotencyProcessingTTL = 2 * time.Minute
	idempotencyResultTTL     = 24 * time.Hour
)

var claimIdempotency = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current then return current end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return ''
`)

var completeIdempotency = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then return 0 end
local expected = cjson.decode(current)
if expected.state ~= 'processing' or expected.fingerprint ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

var renewIdempotency = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then return 0 end
local expected = cjson.decode(current)
if expected.state ~= 'processing' or expected.fingerprint ~= ARGV[1] then return 0 end
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return 1
`)

var markIdempotencyUnreplayable = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then return 0 end
local expected = cjson.decode(current)
if expected.state ~= 'processing' or expected.fingerprint ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

// IdempotencyResult is the replayable HTTP result kept for an idempotent request.
type IdempotencyResult struct {
	State       string `json:"state"`
	Fingerprint string `json:"fingerprint"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

func IdempotencyFingerprint(method, requestURI, contentType string, body []byte) string {
	h := sha256.New()
	writeFingerprintPart(h, method)
	writeFingerprintPart(h, requestURI)
	writeFingerprintPart(h, contentType)
	writeFingerprintPart(h, string(body))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:])
}

func writeFingerprintPart(h hash.Hash, value string) {
	_, _ = h.Write([]byte(strconv.Itoa(len(value))))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{'|'})
}

func idempotencyRedisKey(keyID uint64, idempotencyKey string) string {
	return "idempotency:" + strconv.FormatUint(keyID, 10) + ":" + idempotencyKey
}

// ClaimIdempotency atomically claims a key. Existing state is returned unchanged.
func (r *Repository) ClaimIdempotency(ctx context.Context, keyID uint64, idempotencyKey, fingerprint string) (*IdempotencyResult, bool, error) {
	if r.Redis == nil {
		return nil, false, errors.New("idempotency Redis client is unavailable")
	}
	initial, err := json.Marshal(IdempotencyResult{State: "processing", Fingerprint: fingerprint})
	if err != nil {
		return nil, false, err
	}
	value, err := claimIdempotency.Run(ctx, r.Redis, []string{idempotencyRedisKey(keyID, idempotencyKey)}, initial, idempotencyProcessingTTL.Milliseconds()).Text()
	if err != nil {
		return nil, false, err
	}
	if value == "" {
		return &IdempotencyResult{State: "processing", Fingerprint: fingerprint}, true, nil
	}
	var result IdempotencyResult
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, false, fmt.Errorf("invalid idempotency state: %w", err)
	}
	return &result, false, nil
}

func (r *Repository) CompleteIdempotency(ctx context.Context, keyID uint64, idempotencyKey string, result IdempotencyResult) error {
	if r.Redis == nil {
		return errors.New("idempotency Redis client is unavailable")
	}
	result.State = "done"
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	completed, err := completeIdempotency.Run(ctx, r.Redis, []string{idempotencyRedisKey(keyID, idempotencyKey)}, result.Fingerprint, payload, idempotencyResultTTL.Milliseconds()).Int()
	if err != nil {
		return err
	}
	if completed != 1 {
		return errors.New("idempotency claim is no longer owned")
	}
	return nil
}

// RenewIdempotency extends only the still-processing claim owned by fingerprint.
// A completed result, expired claim, or different request is never renewed.
func (r *Repository) RenewIdempotency(ctx context.Context, keyID uint64, idempotencyKey, fingerprint string) (bool, error) {
	if r.Redis == nil {
		return false, errors.New("idempotency Redis client is unavailable")
	}
	value, err := renewIdempotency.Run(ctx, r.Redis, []string{idempotencyRedisKey(keyID, idempotencyKey)}, fingerprint, idempotencyProcessingTTL.Milliseconds()).Int()
	return value == 1, err
}

// MarkIdempotencyUnreplayable keeps an oversized or uncacheable response from
// being executed again with the same key during the result retention period.
func (r *Repository) MarkIdempotencyUnreplayable(ctx context.Context, keyID uint64, idempotencyKey, fingerprint string) error {
	if r.Redis == nil {
		return errors.New("idempotency Redis client is unavailable")
	}
	result := IdempotencyResult{State: "unreplayable", Fingerprint: fingerprint}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	marked, err := markIdempotencyUnreplayable.Run(ctx, r.Redis, []string{idempotencyRedisKey(keyID, idempotencyKey)}, fingerprint, payload, idempotencyResultTTL.Milliseconds()).Int()
	if err != nil {
		return err
	}
	if marked != 1 {
		return errors.New("idempotency claim is no longer owned")
	}
	return nil
}

// GetIdempotency reads an existing state without claiming the key or changing its TTL.
func (r *Repository) GetIdempotency(ctx context.Context, keyID uint64, idempotencyKey string) (*IdempotencyResult, bool, error) {
	if r.Redis == nil {
		return nil, false, errors.New("idempotency Redis client is unavailable")
	}
	value, err := r.Redis.Get(ctx, idempotencyRedisKey(keyID, idempotencyKey)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var result IdempotencyResult
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, false, fmt.Errorf("invalid idempotency state: %w", err)
	}
	return &result, true, nil
}

func (r *Repository) DeleteIdempotency(ctx context.Context, keyID uint64, idempotencyKey string) error {
	if r.Redis == nil {
		return errors.New("idempotency Redis client is unavailable")
	}
	return r.Redis.Del(ctx, idempotencyRedisKey(keyID, idempotencyKey)).Err()
}

// ValidIdempotencyKey accepts a bounded printable token suitable for a Redis key.
func ValidIdempotencyKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if r <= ' ' || r == ':' || r == '/' || r == '\\' || r > '~' {
			return false
		}
	}
	return true
}
