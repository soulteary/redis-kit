package client_test

import (
	"context"
	"fmt"

	"github.com/soulteary/redis-kit/client"
	"github.com/soulteary/redis-kit/testutil"
)

// CheckHealth reports reachability and latency without the caller having to
// decide what counts as healthy.
func ExampleCheckHealth() {
	rdb, _ := testutil.NewMockRedisClient() // in real code, your own client
	defer func() { _ = rdb.Close() }()

	status := client.CheckHealth(context.Background(), rdb)

	fmt.Println("healthy:", status.Healthy)
	fmt.Println("error:", status.Error)
	// Output:
	// healthy: true
	// error: <nil>
}

// A nil client is reported rather than dereferenced, including the typed nil
// a caller gets from an unassigned field.
func ExampleCheckHealth_nilClient() {
	status := client.CheckHealth(context.Background(), nil)

	fmt.Println("healthy:", status.Healthy)
	fmt.Println("error:", status.Error)
	// Output:
	// healthy: false
	// error: redis client is nil
}

// NewUniversalClient builds whichever client the configuration describes: a
// Sentinel-backed failover client when MasterName is set, a cluster client
// for more than one address, and a single-node client otherwise. Compiled but
// not run: there is nothing here to connect to.
func ExampleNewUniversalClient() {
	cluster, err := client.NewUniversalClient(
		client.DefaultConfig().WithAddrs("10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"),
	)
	if err != nil {
		return
	}
	defer func() { _ = client.Close(cluster) }()

	sentinel, err := client.NewUniversalClient(
		client.DefaultConfig().
			WithAddrs("10.0.0.1:26379", "10.0.0.2:26379").
			WithMasterName("mymaster").
			WithSentinelAuth("sentinel-user", "sentinel-pass"),
	)
	if err != nil {
		return
	}
	defer func() { _ = client.Close(sentinel) }()
}
