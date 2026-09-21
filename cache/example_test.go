package cache_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soulteary/redis-kit/cache"
	"github.com/soulteary/redis-kit/testutil"
)

type user struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// The common case: JSON round-trip behind a key prefix.
func Example() {
	client, _ := testutil.NewMockRedisClient() // in real code, your own client
	defer func() { _ = client.Close() }()

	c := cache.NewCache(client, "app:")
	ctx := context.Background()

	if err := c.Set(ctx, "user:1", user{Name: "ada", Age: 36}, time.Minute); err != nil {
		fmt.Println("set:", err)
		return
	}

	var got user
	if err := c.Get(ctx, "user:1", &got); err != nil {
		fmt.Println("get:", err)
		return
	}

	fmt.Printf("%s is %d\n", got.Name, got.Age)
	// Output:
	// ada is 36
}

// A miss is a sentinel, not a string to match on. It wraps redis.Nil too, so
// code already branching on redis.Nil keeps working.
func ExampleErrKeyNotFound() {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	c := cache.NewCache(client, "app:")

	var got user
	err := c.Get(context.Background(), "absent", &got)

	fmt.Println(errors.Is(err, cache.ErrKeyNotFound))
	fmt.Println(errors.Is(err, redis.Nil))
	// Output:
	// true
	// true
}

// NewCache takes a redis.UniversalClient, so a cluster, a ring or a
// Sentinel-backed failover client goes in exactly where a standalone client
// does. This example is compiled but not run: there is no cluster here to
// talk to.
func ExampleNewCache_clusterClient() {
	cluster := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"},
	})
	defer func() { _ = cluster.Close() }()

	c := cache.NewCache(cluster, "app:")
	_ = c.Set(context.Background(), "user:1", user{Name: "ada", Age: 36}, time.Minute)

	failover := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    "mymaster",
		SentinelAddrs: []string{"10.0.0.1:26379", "10.0.0.2:26379"},
	})
	defer func() { _ = failover.Close() }()

	_ = cache.NewCache(failover, "app:")
}
