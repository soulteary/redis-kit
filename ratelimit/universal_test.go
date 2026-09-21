package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// The constructors take a redis.UniversalClient, so every go-redis client
// shape works. Sentinel is covered by the first two: redis.NewFailoverClient
// returns a *redis.Client and redis.NewFailoverClusterClient a
// *redis.ClusterClient.
var (
	_ redis.UniversalClient = (*redis.Client)(nil)
	_ redis.UniversalClient = (*redis.ClusterClient)(nil)
	_ redis.UniversalClient = (*redis.Ring)(nil)
)

// TestNewRateLimiterAcceptsEveryClientShape compiles only because the
// constructor accepts each shape, and passes only because a typed nil -- a
// NON-nil interface holding a nil pointer, which client == nil misses -- is
// reported instead of dereferenced.
func TestNewRateLimiterAcceptsEveryClientShape(t *testing.T) {
	var (
		standalone *redis.Client
		cluster    *redis.ClusterClient
		ring       *redis.Ring
	)
	shapes := map[string]redis.UniversalClient{
		"nil interface":        nil,
		"*redis.Client":        standalone,
		"*redis.ClusterClient": cluster,
		"*redis.Ring":          ring,
	}

	for name, client := range shapes {
		t.Run(name, func(t *testing.T) {
			limiter := NewRateLimiter(client)
			ctx := context.Background()

			const want = "redis client is nil"
			if _, _, _, err := limiter.CheckLimit(ctx, "key", 10, time.Minute); err == nil || err.Error() != want {
				t.Errorf("CheckLimit() error = %v, want %q", err, want)
			}
			if _, _, err := limiter.CheckCooldown(ctx, "key", time.Minute); err == nil || err.Error() != want {
				t.Errorf("CheckCooldown() error = %v, want %q", err, want)
			}
		})
	}
}
