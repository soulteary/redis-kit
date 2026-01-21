# redis-kit

A unified Redis utility library for Go projects. This package provides common Redis operations including client management, distributed locking, rate limiting, and caching.

## Features

- **Client Management**: Unified Redis client initialization and configuration
- **Distributed Locking**: Redis-based distributed locks with automatic fallback to local locks
- **Rate Limiting**: Flexible rate limiting with support for user/IP/destination-based limits
- **Caching**: Generic cache interface with Redis implementation
- **Health Checks**: Built-in health check functionality

## Installation

```bash
go get github.com/soulteary/redis-kit
```

## Usage

### Client Management

```go
import (
    "github.com/soulteary/redis-kit/client"
    "github.com/redis/go-redis/v9"
)

// Create a client with default configuration
client, err := client.NewClientWithDefaults("localhost:6379")
if err != nil {
    log.Fatal(err)
}
defer client.Close(client)

// Or use custom configuration
cfg := client.DefaultConfig().
    WithAddr("localhost:6379").
    WithPassword("mypassword").
    WithDB(0).
    WithPoolSize(20)

client, err := client.NewClient(cfg)
```

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

// Or use hybrid locker (auto-fallback to local lock)
hybridLocker := lock.NewHybridLocker(client)
success, err := hybridLocker.Lock("my-lock-key")
```

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

// Get a value
var retrievedUser User
err := c.Get(ctx, "user:123", &retrievedUser)

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
├── client/          # Client initialization and management
├── lock/            # Distributed locking
├── ratelimit/       # Rate limiting
├── cache/           # Generic caching interface
└── utils/           # Utility functions
```

## License

See LICENSE file for details.
