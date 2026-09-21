package lock

import (
	"context"
	"errors"
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

// A RedisLocker built from any of them is still a Locker.
var _ Locker = (*RedisLocker)(nil)
var _ Locker = (*HybridLocker)(nil)
var _ Locker = (*LocalLocker)(nil)

// nilClientShapes are the typed nils a caller hands over by accident: an
// unassigned field, or a constructor that returned early. Each is a NON-nil
// interface holding a nil pointer, which a client == nil guard waves through
// to a nil dereference.
func nilClientShapes() map[string]redis.UniversalClient {
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

// TestRedisLockerAcceptsEveryClientShape compiles only because the
// constructors accept each shape, and passes only because a typed nil is
// reported rather than dereferenced.
func TestRedisLockerAcceptsEveryClientShape(t *testing.T) {
	for name, client := range nilClientShapes() {
		t.Run(name, func(t *testing.T) {
			locker := NewRedisLocker(client)

			const want = "redis client is nil"
			if _, err := locker.Lock("key"); err == nil || err.Error() != want {
				t.Errorf("Lock() error = %v, want %q", err, want)
			}
			if err := locker.Unlock("key"); err == nil || err.Error() != want {
				t.Errorf("Unlock() error = %v, want %q", err, want)
			}

			// Acquire classifies it, so a caller failing closed on
			// ErrRedisUnavailable also catches a missing client.
			handle, err := locker.Acquire(context.Background(), "key", time.Minute)
			if handle != nil {
				t.Errorf("Acquire() handle = %v, want nil", handle)
			}
			if err == nil {
				t.Fatal("Acquire() error = nil, want a wrapped ErrRedisUnavailable")
			}
			if !errors.Is(err, ErrRedisUnavailable) {
				t.Errorf("Acquire() error = %v, want it to wrap ErrRedisUnavailable", err)
			}
		})
	}
}

// TestHybridLockerTreatsTypedNilAsNoRedis is the regression test for the
// interface conversion. NewHybridLocker used to take a *redis.Client and test
// client != nil; with an interface parameter that test is true for an
// unassigned *redis.Client field, which would have built a Redis-backed locker
// over a nil pointer and panicked on the first Lock. It must keep behaving
// exactly as it does for an untyped nil: a working process-local lock.
func TestHybridLockerTreatsTypedNilAsNoRedis(t *testing.T) {
	for name, client := range nilClientShapes() {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				kind   string
				locker *HybridLocker
			}{
				{"NewHybridLocker", NewHybridLocker(client)},
				{"NewHybridLockerWithLocalFallback", NewHybridLockerWithLocalFallback(client)},
			} {
				t.Run(tc.kind, func(t *testing.T) {
					if tc.locker.redisLocker != nil {
						t.Fatal("a nil client should leave the hybrid locker with no Redis backend")
					}

					ok, err := tc.locker.Lock("key")
					if err != nil {
						t.Fatalf("Lock() error = %v, want nil", err)
					}
					if !ok {
						t.Fatal("Lock() = false, want true from the local lock")
					}

					// Held locally, so a second acquisition is refused.
					if ok, err := tc.locker.Lock("key"); err != nil || ok {
						t.Errorf("second Lock() = (%v, %v), want (false, nil)", ok, err)
					}

					if err := tc.locker.Unlock("key"); err != nil {
						t.Errorf("Unlock() error = %v, want nil", err)
					}
				})
			}
		})
	}
}
