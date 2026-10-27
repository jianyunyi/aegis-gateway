package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Compare decimal strings to preserve int64 precision in Redis Lua. Delayed
// projections must never overwrite a newer committed total.
var projectQuota = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
local incoming = ARGV[1]
if not current or #current < #incoming or (#current == #incoming and current < incoming) then
  redis.call('SET', KEYS[1], incoming)
else
  -- Touch even an unchanged value to invalidate a reconciler's WATCH.
  redis.call('SET', KEYS[1], current)
end
return 1
`)

var reserveQuota = redis.NewScript(`
local booking = KEYS[1]
local used_key = KEYS[2]
local reserved_key = KEYS[3]
if redis.call('EXISTS', booking) == 1 then
  return 1
end
local base = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local amount = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local used_raw = redis.call('GET', used_key)
local used = base
if used_raw then
  used = tonumber(used_raw)
  if used < base then
    used = base
    redis.call('SET', used_key, used)
  end
else
  redis.call('SET', used_key, used)
end
local reserved = tonumber(redis.call('GET', reserved_key) or '0')
if used + reserved + amount > limit then
  return 0
end
redis.call('INCRBY', reserved_key, amount)
redis.call('SET', booking, amount, 'PX', ttl)
redis.call('PEXPIRE', reserved_key, ttl * 2)
return 1
`)

var releaseQuota = redis.NewScript(`
local amount = redis.call('GET', KEYS[1])
if not amount then
  return 0
end
local remaining = redis.call('DECRBY', KEYS[2], tonumber(amount))
if remaining <= 0 then
  redis.call('DEL', KEYS[2])
end
redis.call('DEL', KEYS[1])
return 1
`)

// ProjectQuota raises the fast Redis counter to at least the committed SQL total.
func (r *Repository) ProjectQuota(ctx context.Context, keyID uint64, tokens int64) error {
	return projectQuota.Run(ctx, r.Redis, []string{"quota:" + strconv.FormatUint(keyID, 10)}, strconv.FormatInt(tokens, 10)).Err()
}

// ReserveQuota atomically admits estimated token usage against committed and
// in-flight usage. A reservation expires if a process dies before settlement.
func (r *Repository) ReserveQuota(ctx context.Context, keyID uint64, requestID string, used, limit, amount int64, ttl time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	if amount < 1 {
		amount = 1
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	id := strconv.FormatUint(keyID, 10)
	result, err := reserveQuota.Run(ctx, r.Redis, []string{
		"quota:reservation:" + id + ":" + requestID,
		"quota:" + id,
		"quota:reserved:" + id,
	}, used, limit, amount, ttl.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// ReleaseQuota removes a request's in-flight reservation exactly once.
func (r *Repository) ReleaseQuota(ctx context.Context, keyID uint64, requestID string) error {
	id := strconv.FormatUint(keyID, 10)
	return releaseQuota.Run(ctx, r.Redis, []string{
		"quota:reservation:" + id + ":" + requestID,
		"quota:reserved:" + id,
	}).Err()
}
