package ratelimit_test

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soulteary/redis-kit/ratelimit"
	"github.com/soulteary/redis-kit/testutil"
)

// A fixed window counted in Redis, so the limit holds across every instance
// of the service rather than per process.
func ExampleRateLimiter_CheckLimit() {
	client, _ := testutil.NewMockRedisClient() // in real code, your own client
	defer func() { _ = client.Close() }()

	limiter := ratelimit.NewRateLimiter(client)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "user:1", 3, time.Minute)
		if err != nil {
			fmt.Println("check:", err)
			return
		}
		fmt.Printf("request %d: allowed=%v remaining=%d\n", i+1, allowed, remaining)
	}
	// Output:
	// request 1: allowed=true remaining=2
	// request 2: allowed=true remaining=1
	// request 3: allowed=true remaining=0
	// request 4: allowed=false remaining=0
}

// A cooldown is the "one message per interval" rule: the first call takes it,
// the rest are refused until it elapses.
func ExampleRateLimiter_CheckCooldown() {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := ratelimit.NewRateLimiter(client)
	ctx := context.Background()

	first, _, err := limiter.CheckCooldown(ctx, "sms:+10000000000", time.Minute)
	if err != nil {
		fmt.Println("cooldown:", err)
		return
	}
	second, _, _ := limiter.CheckCooldown(ctx, "sms:+10000000000", time.Minute)

	fmt.Println("first:", first)
	fmt.Println("second:", second)
	// Output:
	// first: true
	// second: false
}

// NewRateLimiter takes a redis.UniversalClient. Both scripts are single-key,
// so they run unchanged on a cluster -- nothing here is cross-slot. Compiled
// but not run: there is no cluster here.
func ExampleNewRateLimiter_clusterClient() {
	cluster := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"},
	})
	defer func() { _ = cluster.Close() }()

	limiter := ratelimit.NewRateLimiter(cluster)
	_, _, _, _ = limiter.CheckIPLimit(context.Background(), "203.0.113.7", 100, time.Minute)
}
