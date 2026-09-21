# redis-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/redis-kit.svg)](https://pkg.go.dev/github.com/soulteary/redis-kit)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/redis-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/redis-kit)

[中文文档](README_CN.md)

A unified Redis utility library for Go projects. This package provides common Redis operations including client management, distributed locking, rate limiting, and caching.

## Features

- **Any client shape**: every helper takes a `redis.UniversalClient`, so standalone, Cluster, Sentinel and Ring all work
- **Client Management**: Unified Redis client initialization and configuration
- **Distributed Locking**: Redis-based distributed locks, with an opt-in fallback to local locks
- **Rate Limiting**: Flexible rate limiting with support for user/IP/destination-based limits
- **Caching**: Generic cache interface with Redis implementation
- **Health Checks**: Built-in health check functionality

## Installation

```bash
go get github.com/soulteary/redis-kit
```

## Which client do I pass?

Whichever one you have. Every constructor and helper takes a
`redis.UniversalClient`, so all four go-redis client shapes go in the same
slot — no wrapper, no type assertion:

```go
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
rdb := redis.NewClusterClient(&redis.ClusterOptions{Addrs: addrs})
rdb := redis.NewFailoverClient(&redis.FailoverOptions{MasterName: "mymaster", SentinelAddrs: addrs})
rdb := redis.NewRing(&redis.RingOptions{Addrs: shards})

c := cache.NewCache(rdb, "app:")
l := lock.NewRedisLocker(rdb)
r := ratelimit.NewRateLimiter(rdb)
```

No package here issues a multi-key or cross-slot command, so all of them run
unchanged on a cluster. What a cluster does *not* give you is a safer lock: a
key lives on one master, replication is asynchronous, and a failover can lose
an acquisition that was already acknowledged. A single-master lock is only as
safe as the failover behind it.

## Usage

### Client Management

```go
import (
    "github.com/soulteary/redis-kit/client"
    "github.com/redis/go-redis/v9"
)

// Create a client with default configuration
rdb, err := client.NewClientWithDefaults("localhost:6379")
if err != nil {
    log.Fatal(err)
}
defer client.Close(rdb)

// Or use custom configuration
cfg := client.DefaultConfig().
    WithAddr("localhost:6379").
    WithPassword("mypassword").
    WithDB(0).
    WithPoolSize(20)

rdb, err := client.NewClient(cfg)
```

For a cluster or a Sentinel deployment, `NewUniversalClient` builds whichever
client the configuration describes — a failover client when `MasterName` is
set, a cluster client for more than one address, a single-node client
otherwise:

```go
// Redis Cluster
rdb, err := client.NewUniversalClient(
    client.DefaultConfig().WithAddrs("10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"),
)

// Sentinel. WithSentinelAuth is for the Sentinel nodes themselves, which
// usually do not share credentials with the Redis server behind them.
rdb, err := client.NewUniversalClient(
    client.DefaultConfig().
        WithAddrs("10.0.0.1:26379", "10.0.0.2:26379").
        WithMasterName("mymaster").
        WithSentinelAuth("sentinel-user", "sentinel-pass"),
)
```

`NewClient` still returns a concrete `*redis.Client`, for when you need
`Options()` or something that insists on the concrete type.

### Distributed Locking

```go
import "github.com/soulteary/redis-kit/lock"

// Create a Redis locker
locker := lock.NewRedisLocker(client)

// Acquire a lock
success, err := locker.Lock("my-lock-key")
if err != nil {
    log.Fatal(err)
}
if !success {
    log.Println("Lock already held")
    return
}

// Do work...

// Release the lock
defer locker.Unlock("my-lock-key")

// Or use hybrid locker. It uses Redis when a client is given and a
// process-local lock when client is nil. When Redis is merely FAILING it
// returns lock.ErrRedisUnavailable rather than degrading, so the caller can
// fail closed:
hybridLocker := lock.NewHybridLocker(client)

// Opt back in to the old degrade-to-local behaviour only where a second
// concurrent holder is acceptable -- a cache warmer, never a payment:
hybridLocker = lock.NewHybridLockerWithLocalFallback(client)
success, err := hybridLocker.Lock("my-lock-key")
```

**Notes**
- `Unlock` requires the same process to hold the lock value; unlocking a key without a local lock value returns an error to avoid deleting someone else's lock.
- `HybridLocker` does **not** fall back to a local lock when Redis fails. A process-local lock provides no mutual exclusion between instances, so degrading to it during an outage drops the guarantee at exactly the moment it matters, and the caller cannot tell the difference from the return value. `NewHybridLocker` returns `lock.ErrRedisUnavailable` instead.
- `NewHybridLockerWithLocalFallback` restores the degrading behaviour explicitly. Mutual exclusion across instances is lost while the fallback is in effect, so use it only where a second concurrent holder is acceptable. A key held through the fallback is not handed out via Redis again until it is released, and its unlock is routed back to the local lock.

#### Handle API: a lock you can renew

`Lock`/`Unlock` identify a lock by key alone, which is why `Unlock` has to check
that this process still holds the lease. `Acquire` hands you the token instead, so
`Release` and `Extend` act on **that specific acquisition**:

```go
locker := lock.NewRedisLocker(client)

handle, err := locker.Acquire(ctx, "my-lock-key", 30*time.Second)
if err != nil {
    return err // includes "already held" — check errors.Is below
}
defer handle.Release(ctx)

log.Printf("holding %s until %s", handle.Key(), handle.ExpiresAt())

// Renew before the lease elapses, for work that outlives it
if err := handle.Extend(ctx, 30*time.Second); err != nil {
    return err // the lease is gone; stop doing the work it protected
}
```

Without `Extend` the TTL is fixed at `lock.DefaultLockTime` (15s) and work that
outlives the lease simply loses the lock, with no error anywhere.

#### Lock errors

| Sentinel | Meaning |
|----------|---------|
| `ErrRedisUnavailable` | Redis failed. `NewHybridLocker` returns this instead of degrading to a local lock — fail closed. |
| `ErrLockExpired` | The lease elapsed before `Unlock`/`Release`. No delete was issued, so a later holder's lock is untouched. |
| `ErrLockNotHeld` | This process holds no lock value for the key. |
| `ErrLockValueMismatch` | The stored value is not this process's token. |
| `ErrLockValueType` | The stored value is not the expected type. |
| `ErrLockTrackingLimit` | The process-wide lock map is full. Prefer `Acquire`/`Handle`, which needs no map. |

Match them with `errors.Is`.

An `Unlock` whose lease has already elapsed reports `ErrLockExpired` **without
issuing the delete**, so it cannot release a key a later holder has since
acquired. Tracked entries are swept periodically, so a process that acquires and
never releases — panic, early return, expiry — does not grow the map without
bound.

### Rate Limiting

```go
import (
    "github.com/soulteary/redis-kit/ratelimit"
    "time"
)

// Create a rate limiter
limiter := ratelimit.NewRateLimiter(client)

// Check rate limit
allowed, remaining, resetTime, err := limiter.CheckLimit(
    ctx,
    "user:123",
    10,                    // limit: 10 requests
    1 * time.Hour,         // window: 1 hour
)

// Check cooldown
allowed, resetTime, err := limiter.CheckCooldown(
    ctx,
    "challenge:abc",
    60 * time.Second,      // cooldown: 60 seconds
)

// Convenience methods
allowed, remaining, resetTime, err := limiter.CheckUserLimit(ctx, "user123", 10, time.Hour)
allowed, remaining, resetTime, err := limiter.CheckIPLimit(ctx, "192.168.1.1", 5, time.Minute)
allowed, remaining, resetTime, err := limiter.CheckDestinationLimit(ctx, "user@example.com", 10, time.Hour)
```

**Notes**
- Rate limiting and cooldown checks use Redis Lua scripts (`EVAL`) to ensure atomicity; make sure scripts are allowed in your Redis deployment.

`CheckLimit` validates `limit`: a limit of `0` is rejected rather than treated
as "no counter yet", which used to admit the first request of every window — so
"allow nothing" let traffic through.

### Caching

```go
import "github.com/soulteary/redis-kit/cache"

// Create a cache with key prefix
c := cache.NewCache(client, "myapp:")

// Set a value
type User struct {
    ID   string
    Name string
}
user := User{ID: "123", Name: "Alice"}
err := c.Set(ctx, "user:123", user, 1*time.Hour)

// Get a value. A miss is ErrKeyNotFound, which also matches redis.Nil.
var retrievedUser User
if err := c.Get(ctx, "user:123", &retrievedUser); err != nil {
    if errors.Is(err, cache.ErrKeyNotFound) || errors.Is(err, redis.Nil) {
        // not cached
    }
    return err
}

// Check existence
exists, err := c.Exists(ctx, "user:123")

// Delete
err := c.Del(ctx, "user:123")

// Get TTL
ttl, err := c.TTL(ctx, "user:123")

// Set expiration
err := c.Expire(ctx, "user:123", 2*time.Hour)
```

### Health Checks

```go
import "github.com/soulteary/redis-kit/client"

// Simple health check
healthy := client.HealthCheck(ctx, client)

// Detailed health status
status := client.CheckHealth(ctx, client)
if !status.Healthy {
    log.Printf("Redis unhealthy: %v (latency: %v)", status.Error, status.Latency)
}
```

## Project Structure

```
redis-kit/
├── doc.go           # Module-level documentation; no code
├── client/          # Client initialization and management
├── lock/            # Distributed locking
├── ratelimit/       # Rate limiting
├── cache/           # Generic caching interface
├── utils/           # Utility functions (the only package not importing go-redis)
├── testutil/        # Testing utilities (mock Redis)
└── internal/        # Not importable; shared guards
```

One module, one package per concern. Module graph pruning keeps a requirement
none of your imported packages needs out of your `go.mod` and `go.sum`, so the
packages you skip cost you nothing.

## Requirements

- **Go 1.27+** (`go.mod` declares `go 1.27.0`)
- Redis server (optional for testing, mock Redis is provided)

## Test Coverage

The library includes comprehensive tests with mock Redis support, so you can run tests without a real Redis instance:

```bash
# Run all tests
go test ./... -v

# Run tests with coverage
go test ./... -coverprofile=coverage.out -covermode=atomic

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# View coverage summary
go tool cover -func=coverage.out
```

## Examples

### Complete Example: Rate-Limited Cache with Locking

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/soulteary/redis-kit/cache"
    "github.com/soulteary/redis-kit/client"
    "github.com/soulteary/redis-kit/lock"
    "github.com/soulteary/redis-kit/ratelimit"
)

func main() {
    ctx := context.Background()
    
    // Initialize Redis client
    redisClient, err := client.NewClientWithDefaults("localhost:6379")
    if err != nil {
        panic(err)
    }
    defer redisClient.Close()
    
    // Health check
    if !client.HealthCheck(ctx, redisClient) {
        panic("Redis is not healthy")
    }
    
    // Create cache
    userCache := cache.NewCache(redisClient, "user:")
    
    // Create locker
    locker := lock.NewHybridLocker(redisClient)
    
    // Create rate limiter
    limiter := ratelimit.NewRateLimiter(redisClient)
    
    // Example: Get user with caching and rate limiting
    userID := "user123"
    
    // Check rate limit
    allowed, remaining, resetTime, err := limiter.CheckUserLimit(ctx, userID, 10, time.Hour)
    if err != nil {
        panic(err)
    }
    if !allowed {
        fmt.Printf("Rate limit exceeded. Reset at: %v\n", resetTime)
        return
    }
    fmt.Printf("Rate limit OK. Remaining: %d\n", remaining)
    
    // Try to acquire lock
    lockKey := fmt.Sprintf("user:%s:lock", userID)
    acquired, err := locker.Lock(lockKey)
    if err != nil {
        panic(err)
    }
    if !acquired {
        fmt.Println("Could not acquire lock")
        return
    }
    defer locker.Unlock(lockKey)
    
    // Check cache first
    type User struct {
        ID   string
        Name string
    }
    var user User
    exists, err := userCache.Exists(ctx, userID)
    if err != nil {
        panic(err)
    }
    
    if exists {
        // Cache hit
        err = userCache.Get(ctx, userID, &user)
        if err != nil {
            panic(err)
        }
        fmt.Printf("Cache hit: %+v\n", user)
    } else {
        // Cache miss - fetch from database
        user = User{ID: userID, Name: "Alice"}
        
        // Store in cache
        err = userCache.Set(ctx, userID, user, 1*time.Hour)
        if err != nil {
            panic(err)
        }
        fmt.Printf("Cached: %+v\n", user)
    }
}
```

## Changelog

Release-by-release detail, with the measured numbers behind each claim, lives
in [CHANGELOG.md](CHANGELOG.md).

## Upgrade Notes (v1.7.0)

**Client parameters are now `redis.UniversalClient` instead of
`*redis.Client`.** Existing code keeps compiling — `*redis.Client` satisfies
the interface, so every call that worked before still works and no import
changes. What is new is that a Cluster, Sentinel or Ring client now goes in
the same slot; before, the compiler simply refused the call.

- **Eleven functions changed parameter type**: `cache.NewCache`;
  `client.Ping`, `Close`, `HealthCheck`, `CheckHealth`; `lock.NewRedisLocker`,
  `NewRedisLockerWithLockTime`, `NewHybridLocker`,
  `NewHybridLockerWithLocalFallback`; `ratelimit.NewRateLimiter`,
  `NewRateLimiterWithPrefixes`. The only usage that breaks is taking one as a
  value at its old concrete type, e.g.
  `var f func(*redis.Client, string) *cache.RedisCache = cache.NewCache`.
- **Your dependency footprint does not change.** No new module, nothing added
  to `go.sum`, no new minimum version pushed onto you through MVS. Measured
  for a program importing all four packages: same 13 modules and 22 `go.sum`
  lines, one extra linked package, and a binary 0.25% larger.
- **A typed nil is now handled.** An unassigned `*redis.Client` field becomes a
  *non-nil* interface holding a nil pointer, which a `client == nil` check
  misses — so every guard here looks inside the interface and still reports
  `redis client is nil`. `NewHybridLocker` treats it as "no Redis" and gives
  you the process-local lock, exactly as it does for an untyped nil.
- **`client.NewUniversalClient` is new**, along with `Config.Addrs`,
  `MasterName`, `SentinelUsername` and `SentinelPassword`. `NewClient` is
  unchanged and still returns a concrete `*redis.Client`.
- **A zero `DialTimeout` no longer fails every connection.** A hand-built
  `Config{Addr: "..."}` produced an already-expired context, so the connection
  test failed with `context deadline exceeded` before a packet was sent. It
  now falls back to the documented 5s default.

## Upgrade Notes (v1.6.0)

**`HybridLocker` no longer degrades to a local lock.** That is the change most
likely to surface in a deployment, and it is deliberate.

- **A Redis failure returns `ErrRedisUnavailable` instead of a local lock.**
  `HybridLocker.Lock` tried Redis and fell through to `LocalLocker` on *any*
  error. A process-local lock provides no exclusion between instances, so during
  a Redis outage every instance took its own "lock" and believed it held the key
  — and the caller saw `(true, nil)`, indistinguishable from a real distributed
  lock. The guarantee disappeared at exactly the moment it mattered. **Handle
  `ErrRedisUnavailable` and fail closed**, or use
  `NewHybridLockerWithLocalFallback` to keep the old behaviour where a second
  concurrent holder is genuinely acceptable — a cache warmer, never a payment.
- **A stale holder can no longer release someone else's lock.** The token lived
  in a process-wide map keyed by the lock key, so a second acquisition of the same
  key overwrote the first holder's token and the first holder's `Unlock`
  compare-and-deleted with the *second* holder's token. The map now records the
  lease deadline too, and an `Unlock` past its lease reports `ErrLockExpired`
  without issuing the delete. **An `Unlock` that used to succeed spuriously now
  returns an error** — which is the correct answer.
- **The lock map is swept and bounded.** A process that acquired and never
  released grew it without bound; it now reports `ErrLockTrackingLimit` when full.
- **`Acquire`/`Handle` is new**, and is the better API: it hands the token to the
  caller, so `Release` and `Extend` act on a specific acquisition and need no
  shared map. `Extend` also fills in the missing renewal path — the TTL was fixed
  at 15s with no way to refresh it, so work outliving the lease lost the lock
  silently.
- **A cache miss carries `ErrKeyNotFound`, and matches `redis.Nil`.** `cache.Get`
  returned a plain `fmt.Errorf`, so `errors.Is(err, redis.Nil)` was false and
  callers had to match on the error text. The original message is preserved.
- **`ratelimit.CheckLimit` validates `limit`.** With `limit=0` the Lua script took
  its "no counter yet" branch and admitted the first request of every window, so
  "allow nothing" let traffic through.
- **The License badge said MIT.** The `LICENSE` file is Apache 2.0.
- **Requirements said Go 1.26**; `go.mod` requires `1.27.0`.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Follow Go best practices and conventions
- Add tests for new features
- Ensure all tests pass (`go test ./...`)
- Run `go fmt` and `go vet` before committing
- Update documentation as needed

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
