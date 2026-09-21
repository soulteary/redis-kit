package cache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewCache takes a redis.UniversalClient, so every go-redis client shape works
// -- not just the standalone one it used to insist on. A Sentinel deployment
// needs no separate entry: redis.NewFailoverClient returns a *redis.Client and
// redis.NewFailoverClusterClient a *redis.ClusterClient.
var (
	_ redis.UniversalClient = (*redis.Client)(nil)
	_ redis.UniversalClient = (*redis.ClusterClient)(nil)
	_ redis.UniversalClient = (*redis.Ring)(nil)
)

// clientShapes are the typed nils a caller hands over by accident: an
// unassigned struct field, or a constructor that returned before assigning.
// Every one is a NON-nil interface holding a nil pointer, so a guard written
// as client == nil lets it straight through to a nil dereference.
func clientShapes() map[string]redis.UniversalClient {
	var (
		standalone *redis.Client
		cluster    *redis.ClusterClient
		ring       *redis.Ring
	)
	return map[string]redis.UniversalClient{
		"nil interface":        nil,
		"*redis.Client":        standalone,
		"*redis.ClusterClient": cluster,
		"*redis.Ring":          ring,
	}
}

// TestNewCacheAcceptsEveryClientShape does double duty: it only compiles
// because NewCache accepts each shape, and it only passes because a typed nil
// is reported rather than dereferenced. A panic here is the regression.
func TestNewCacheAcceptsEveryClientShape(t *testing.T) {
	for name, client := range clientShapes() {
		t.Run(name, func(t *testing.T) {
			c := NewCache(client, "test:")
			ctx := context.Background()

			const want = "redis client is nil"

			if err := c.Set(ctx, "key", "value", time.Minute); err == nil || err.Error() != want {
				t.Errorf("Set() error = %v, want %q", err, want)
			}
			var dest string
			if err := c.Get(ctx, "key", &dest); err == nil || err.Error() != want {
				t.Errorf("Get() error = %v, want %q", err, want)
			}
			if err := c.Del(ctx, "key"); err == nil || err.Error() != want {
				t.Errorf("Del() error = %v, want %q", err, want)
			}
			if _, err := c.Exists(ctx, "key"); err == nil || err.Error() != want {
				t.Errorf("Exists() error = %v, want %q", err, want)
			}
			if _, err := c.TTL(ctx, "key"); err == nil || err.Error() != want {
				t.Errorf("TTL() error = %v, want %q", err, want)
			}
			if err := c.Expire(ctx, "key", time.Minute); err == nil || err.Error() != want {
				t.Errorf("Expire() error = %v, want %q", err, want)
			}
		})
	}
}

// RedisCache satisfies the package's own Cache interface no matter which
// client shape built it.
var _ Cache = (*RedisCache)(nil)
