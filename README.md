# redis-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/redis-kit.svg)](https://pkg.go.dev/github.com/soulteary/redis-kit)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/redis-kit)](https://goreportcard.com/report/github.com/soulteary/redis-kit)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

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
├── utils/           # Utility functions
└── testutil/        # Testing utilities (mock Redis)
```

## Requirements

- Go 1.25 or later
- Redis server (optional for testing, mock Redis is provided)

## Testing

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

See [COVERAGE.md](COVERAGE.md) for detailed coverage information.

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

See LICENSE file for details.
