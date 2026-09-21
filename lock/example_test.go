package lock_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soulteary/redis-kit/lock"
	"github.com/soulteary/redis-kit/testutil"
)

// Acquire is the API to prefer. The returned Handle carries the token of this
// acquisition, so releasing it can never affect a lock somebody else took
// after this lease elapsed.
func ExampleRedisLocker_Acquire() {
	client, _ := testutil.NewMockRedisClient() // in real code, your own client
	defer func() { _ = client.Close() }()

	locker := lock.NewRedisLocker(client)
	ctx := context.Background()

	handle, err := locker.Acquire(ctx, "job:nightly", 30*time.Second)
	if err != nil {
		fmt.Println("acquire:", err)
		return
	}
	if handle == nil {
		fmt.Println("someone else holds it")
		return
	}

	// A second acquisition of the same key is contention, not an error:
	// (nil, nil).
	other, err := locker.Acquire(ctx, "job:nightly", 30*time.Second)
	fmt.Println("contended:", other == nil, err == nil)

	// Work outliving the lease renews it rather than losing the lock silently.
	if err := handle.Extend(ctx, 30*time.Second); err != nil {
		fmt.Println("extend:", err)
		return
	}

	fmt.Println("key:", handle.Key())
	fmt.Println("release:", handle.Release(ctx))
	// Output:
	// contended: true true
	// key: job:nightly
	// release: <nil>
}

// A Redis failure is reported, not silently downgraded to a process-local
// lock -- which would provide no mutual exclusion between instances at exactly
// the moment it matters.
func ExampleRedisLocker_Acquire_redisUnavailable() {
	client, mock := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()
	mock.SetShouldFail(true)

	_, err := lock.NewRedisLocker(client).Acquire(context.Background(), "job:nightly", time.Second)

	fmt.Println(errors.Is(err, lock.ErrRedisUnavailable))
	// Output:
	// true
}

// With no client at all, a HybridLocker is a process-local lock. Passing an
// unassigned *redis.Client field counts as "no client" too -- it is a typed
// nil, and locking through it would panic.
func ExampleNewHybridLocker() {
	var unset *redis.Client // never assigned

	locker := lock.NewHybridLocker(unset)

	first, err := locker.Lock("job:nightly")
	fmt.Println("first:", first, err)

	second, err := locker.Lock("job:nightly")
	fmt.Println("second:", second, err)

	fmt.Println("unlock:", locker.Unlock("job:nightly"))
	// Output:
	// first: true <nil>
	// second: false <nil>
	// unlock: <nil>
}

// NewRedisLocker takes a redis.UniversalClient, so a cluster or a
// Sentinel-backed failover client goes in where a standalone client does.
// Compiled but not run: there is no cluster here.
func ExampleNewRedisLocker_clusterClient() {
	cluster := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"},
	})
	defer func() { _ = cluster.Close() }()

	locker := lock.NewRedisLocker(cluster)
	handle, err := locker.Acquire(context.Background(), "job:nightly", 30*time.Second)
	if err != nil || handle == nil {
		return
	}
	defer func() { _ = handle.Release(context.Background()) }()
}
