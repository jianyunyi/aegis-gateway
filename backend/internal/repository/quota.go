package repository

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// Compare decimal strings to preserve int64 precision in Redis Lua. Delayed
// projections must never overwrite a newer committed total. Reconciliation
// remains responsible for repairing an incorrectly high Redis value.
var projectQuota = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
local incoming = ARGV[1]
if not current or #current < #incoming or (#current == #incoming and current < incoming) then
  redis.call('SET', KEYS[1], incoming)
else
  -- Touch even an unchanged value to invalidate a reconciler's WATCH. A
  -- corrupt high counter must not hide a newly committed usage projection.
  redis.call('SET', KEYS[1], current)
end
return 1
`)

func (r *Repository) ProjectQuota(ctx context.Context, keyID uint64, tokens int64) error {
	return projectQuota.Run(ctx, r.Redis, []string{"quota:" + strconv.FormatUint(keyID, 10)}, strconv.FormatInt(tokens, 10)).Err()
}
