# redis-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/redis-kit.svg)](https://pkg.go.dev/github.com/soulteary/redis-kit)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/redis-kit)](https://goreportcard.com/report/github.com/soulteary/redis-kit)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/redis-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/redis-kit)

[English](README.md)

一个统一的 Go Redis 工具库。提供常用的 Redis 操作，包括客户端管理、分布式锁、限流和缓存。

## 功能特性

- **客户端管理** - 统一的 Redis 客户端初始化和配置
- **分布式锁** - 基于 Redis 的分布式锁，支持自动降级到本地锁
- **限流器** - 灵活的限流功能，支持用户/IP/目标地址的限流
- **缓存** - 通用缓存接口，提供 Redis 实现
- **健康检查** - 内置健康检查功能

## 安装

```bash
go get github.com/soulteary/redis-kit
```

## 快速开始

### 客户端管理

```go
import (
    "github.com/soulteary/redis-kit/client"
    "github.com/redis/go-redis/v9"
)

// 使用默认配置创建客户端
client, err := client.NewClientWithDefaults("localhost:6379")
if err != nil {
    log.Fatal(err)
}
defer client.Close(client)

// 或使用自定义配置
cfg := client.DefaultConfig().
    WithAddr("localhost:6379").
    WithPassword("mypassword").
    WithDB(0).
    WithPoolSize(20)

client, err := client.NewClient(cfg)
```

### 分布式锁

```go
import "github.com/soulteary/redis-kit/lock"

// 创建 Redis 锁
locker := lock.NewRedisLocker(client)

// 获取锁
success, err := locker.Lock("my-lock-key")
if err != nil {
    log.Fatal(err)
}
if !success {
    log.Println("锁已被占用")
    return
}

// 执行业务逻辑...

// 释放锁
defer locker.Unlock("my-lock-key")

// 或使用混合锁（自动降级到本地锁）
hybridLocker := lock.NewHybridLocker(client)
success, err := hybridLocker.Lock("my-lock-key")
```

### 限流器

```go
import (
    "github.com/soulteary/redis-kit/ratelimit"
    "time"
)

// 创建限流器
limiter := ratelimit.NewRateLimiter(client)

// 检查限流
allowed, remaining, resetTime, err := limiter.CheckLimit(
    ctx,
    "user:123",
    10,                    // 限制：10 次请求
    1 * time.Hour,         // 窗口：1 小时
)

// 检查冷却时间
allowed, resetTime, err := limiter.CheckCooldown(
    ctx,
    "challenge:abc",
    60 * time.Second,      // 冷却：60 秒
)

// 便捷方法
allowed, remaining, resetTime, err := limiter.CheckUserLimit(ctx, "user123", 10, time.Hour)
allowed, remaining, resetTime, err := limiter.CheckIPLimit(ctx, "192.168.1.1", 5, time.Minute)
allowed, remaining, resetTime, err := limiter.CheckDestinationLimit(ctx, "user@example.com", 10, time.Hour)
```

### 缓存

```go
import "github.com/soulteary/redis-kit/cache"

// 创建带键前缀的缓存
c := cache.NewCache(client, "myapp:")

// 设置值
type User struct {
    ID   string
    Name string
}
user := User{ID: "123", Name: "Alice"}
err := c.Set(ctx, "user:123", user, 1*time.Hour)

// 获取值
var retrievedUser User
err := c.Get(ctx, "user:123", &retrievedUser)

// 检查是否存在
exists, err := c.Exists(ctx, "user:123")

// 删除
err := c.Del(ctx, "user:123")

// 获取 TTL
ttl, err := c.TTL(ctx, "user:123")

// 设置过期时间
err := c.Expire(ctx, "user:123", 2*time.Hour)
```

### 健康检查

```go
import "github.com/soulteary/redis-kit/client"

// 简单健康检查
healthy := client.HealthCheck(ctx, client)

// 详细健康状态
status := client.CheckHealth(ctx, client)
if !status.Healthy {
    log.Printf("Redis 不健康: %v (延迟: %v)", status.Error, status.Latency)
}
```

## 项目结构

```
redis-kit/
├── client/          # 客户端初始化和管理
├── lock/            # 分布式锁
├── ratelimit/       # 限流器
├── cache/           # 通用缓存接口
├── utils/           # 工具函数
└── testutil/        # 测试工具（Mock Redis）
```

## 测试覆盖率

本项目保持较高的测试覆盖率：

| 包 | 覆盖率 |
|------|--------|
| cache | 100.0% |
| client | 98.3% |
| lock | 93.9% |
| ratelimit | 87.0% |
| testutil | 90.1% |
| utils | 100.0% |
| **总计** | **92.3%** |

运行测试并查看覆盖率：

```bash
# 运行所有测试
go test ./... -v

# 运行测试并生成覆盖率报告
go test ./... -coverprofile=coverage.out -covermode=atomic

# 生成 HTML 覆盖率报告
go tool cover -html=coverage.out -o coverage.html

# 查看覆盖率摘要
go tool cover -func=coverage.out
```

## 环境要求

- Go 1.25 或更高版本
- Redis 服务器（测试时可选，提供 Mock Redis）

## 完整示例：带限流的缓存与锁

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
    
    // 初始化 Redis 客户端
    redisClient, err := client.NewClientWithDefaults("localhost:6379")
    if err != nil {
        panic(err)
    }
    defer redisClient.Close()
    
    // 健康检查
    if !client.HealthCheck(ctx, redisClient) {
        panic("Redis 不健康")
    }
    
    // 创建缓存
    userCache := cache.NewCache(redisClient, "user:")
    
    // 创建锁
    locker := lock.NewHybridLocker(redisClient)
    
    // 创建限流器
    limiter := ratelimit.NewRateLimiter(redisClient)
    
    // 示例：获取用户（带缓存和限流）
    userID := "user123"
    
    // 检查限流
    allowed, remaining, resetTime, err := limiter.CheckUserLimit(ctx, userID, 10, time.Hour)
    if err != nil {
        panic(err)
    }
    if !allowed {
        fmt.Printf("超出限流。重置时间: %v\n", resetTime)
        return
    }
    fmt.Printf("限流检查通过。剩余: %d\n", remaining)
    
    // 尝试获取锁
    lockKey := fmt.Sprintf("user:%s:lock", userID)
    acquired, err := locker.Lock(lockKey)
    if err != nil {
        panic(err)
    }
    if !acquired {
        fmt.Println("无法获取锁")
        return
    }
    defer locker.Unlock(lockKey)
    
    // 先检查缓存
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
        // 缓存命中
        err = userCache.Get(ctx, userID, &user)
        if err != nil {
            panic(err)
        }
        fmt.Printf("缓存命中: %+v\n", user)
    } else {
        // 缓存未命中 - 从数据库获取
        user = User{ID: userID, Name: "Alice"}
        
        // 存入缓存
        err = userCache.Set(ctx, userID, user, 1*time.Hour)
        if err != nil {
            panic(err)
        }
        fmt.Printf("已缓存: %+v\n", user)
    }
}
```

## 贡献

欢迎贡献！请随时提交 Pull Request。

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### 开发指南

- 遵循 Go 最佳实践和规范
- 为新功能添加测试
- 确保所有测试通过 (`go test ./...`)
- 提交前运行 `go fmt` 和 `go vet`
- 根据需要更新文档

## 许可证

详见 LICENSE 文件。
