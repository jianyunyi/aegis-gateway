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
local expiry_key = KEYS[4]
local amounts_key = KEYS[5]
local ttl = tonumber(ARGV[4])
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)

local expired = redis.call('ZRANGEBYSCORE', expiry_key, '-inf', now_ms)
for _, expired_booking in ipairs(expired) do
  local expired_amount = redis.call('HGET', amounts_key, expired_booking)
  if expired_amount then
    local remaining = redis.call('DECRBY', reserved_key, expired_amount)
    if tonumber(remaining) <= 0 then
      redis.call('DEL', reserved_key)
    end
  end
  redis.call('DEL', expired_booking)
  redis.call('HDEL', amounts_key, expired_booking)
  redis.call('ZREM', expiry_key, expired_booking)
end
if redis.call('ZCARD', expiry_key) == 0 then
  redis.call('DEL', expiry_key, amounts_key)
end

if redis.call('EXISTS', booking) == 1 then
  return 1
end
local base = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local amount = ARGV[3]
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
if used + reserved + tonumber(amount) > limit then
  return 0
end
redis.call('INCRBY', reserved_key, amount)
redis.call('SET', booking, amount, 'PX', ttl)
local expiry_now = redis.call('TIME')
local expiry_ms = tonumber(expiry_now[1]) * 1000 + math.floor(tonumber(expiry_now[2]) / 1000) + redis.call('PTTL', booking)
redis.call('HSET', amounts_key, booking, amount)
redis.call('ZADD', expiry_key, expiry_ms, booking)

local function extend_ttl(key, desired)
  if redis.call('PTTL', key) < desired then
    redis.call('PEXPIRE', key, desired)
  end
end
local tracking_ttl = ttl * 2
extend_ttl(reserved_key, tracking_ttl)
extend_ttl(expiry_key, tracking_ttl)
extend_ttl(amounts_key, tracking_ttl)
return 1
`)

var releaseQuota = redis.NewScript(`
local amount = redis.call('GET', KEYS[1])
if not amount then
  amount = redis.call('HGET', KEYS[4], KEYS[1])
end
if amount then
  local remaining = redis.call('DECRBY', KEYS[2], amount)
  if tonumber(remaining) <= 0 then
    redis.call('DEL', KEYS[2])
  end
end
redis.call('DEL', KEYS[1])
redis.call('HDEL', KEYS[4], KEYS[1])
redis.call('ZREM', KEYS[3], KEYS[1])
if redis.call('ZCARD', KEYS[3]) == 0 then
  redis.call('DEL', KEYS[3], KEYS[4])
end
return amount and 1 or 0
`)

// ProjectQuota raises the fast Redis counter to at least the committed SQL total.
func (r *Repository) ProjectQuota(ctx context.Context, keyID uint64, tokens int64) error {
	return projectQuota.Run(ctx, r.Redis, []string{"quota:" + strconv.FormatUint(keyID, 10)}, strconv.FormatInt(tokens, 10)).Err()
}

// ReserveQuota atomically admits estimated token usage against committed and
// in-flight usage. Expired abandoned reservations are reclaimed during admission.
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
	booking := "quota:reservation:" + id + ":" + requestID
	result, err := reserveQuota.Run(ctx, r.Redis, []string{
		booking,
		"quota:" + id,
		"quota:reserved:" + id,
		"quota:reservation-expiry:" + id,
		"quota:reservation-amounts:" + id,
	}, used, limit, amount, ttl.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// ReleaseQuota removes a request's in-flight reservation exactly once.
func (r *Repository) ReleaseQuota(ctx context.Context, keyID uint64, requestID string) error {
	id := strconv.FormatUint(keyID, 10)
	booking := "quota:reservation:" + id + ":" + requestID
	return releaseQuota.Run(ctx, r.Redis, []string{
		booking,
		"quota:reserved:" + id,
		"quota:reservation-expiry:" + id,
		"quota:reservation-amounts:" + id,
	}).Err()
}
