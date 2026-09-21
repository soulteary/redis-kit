// Package rediskit is the module root for redis-kit, a set of small,
// independent Redis building blocks for Go services. It holds documentation
// only; the code lives in the subpackages below.
//
// # Layout
//
// Every piece is its own package, so importing one never links the others:
//
//   - github.com/soulteary/redis-kit/cache -- JSON cache with a key prefix.
//   - github.com/soulteary/redis-kit/client -- client construction, Ping and
//     health checks.
//   - github.com/soulteary/redis-kit/lock -- distributed locks, plus a
//     process-local one.
//   - github.com/soulteary/redis-kit/ratelimit -- fixed-window rate limits and
//     cooldowns.
//   - github.com/soulteary/redis-kit/testutil -- an in-memory Redis for tests.
//   - github.com/soulteary/redis-kit/utils -- key building and context
//     timeouts. The only package here that does not import go-redis.
//
// This is a single module. Module graph pruning keeps a requirement that none
// of your imported packages needs out of your go.mod and go.sum, so the
// packages you skip cost you nothing.
//
// # Which client type to pass
//
// Every constructor and helper that needs a Redis connection takes a
// [github.com/redis/go-redis/v9.UniversalClient], never a concrete
// *redis.Client. All four go-redis client shapes satisfy it:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	rdb := redis.NewClusterClient(&redis.ClusterOptions{Addrs: addrs})
//	rdb := redis.NewFailoverClient(&redis.FailoverOptions{MasterName: "mymaster", SentinelAddrs: addrs})
//	rdb := redis.NewRing(&redis.RingOptions{Addrs: shards})
//
//	c := cache.NewCache(rdb, "app:")
//	l := lock.NewRedisLocker(rdb)
//	r := ratelimit.NewRateLimiter(rdb)
//
// [github.com/soulteary/redis-kit/client.NewUniversalClient] builds whichever
// of those a Config describes, so cluster and Sentinel deployments are served
// end to end.
//
// Taking an interface reopens a hole that a *redis.Client parameter closed by
// construction: an unassigned client field is a typed nil, a NON-nil interface
// holding a nil pointer, and a guard written as client == nil lets it through
// to a nil dereference. Every nil check in this module therefore looks inside
// the interface and reports "redis client is nil" instead of panicking.
//
// # Cluster caveats
//
// Nothing here issues a multi-key or cross-slot command, so every package runs
// unchanged on a cluster. What a cluster does not give you is a safer lock: a
// key lives on one master, replication is asynchronous, and a failover can
// lose an acquisition that was already acknowledged. A single-master lock is
// only as safe as the failover behind it, whichever client shape you pass.
package rediskit
