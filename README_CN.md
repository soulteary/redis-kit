# redis-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/redis-kit.svg)](https://pkg.go.dev/github.com/soulteary/redis-kit)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/redis-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/redis-kit)

[English](README.md)

一个统一的 Go Redis 工具库。提供常用的 Redis 操作，包括客户端管理、分布式锁、限流和缓存。

## 功能特性

- **客户端管理** - 统一的 Redis 客户端初始化和配置
- **分布式锁** - 基于 Redis 的分布式锁，可选地降级到本地锁
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

// 或使用混合锁。client 非 nil 时走 Redis，为 nil 时使用进程内本地锁。
// 当 Redis 只是「出故障」时会返回 lock.ErrRedisUnavailable 而不是悄悄降级，
// 以便调用方可以 fail closed：
hybridLocker := lock.NewHybridLocker(client)

// 仅在「允许出现第二个并发持有者」的场景（例如缓存预热，绝不能用于支付）
// 才显式选择旧的降级行为：
hybridLocker = lock.NewHybridLockerWithLocalFallback(client)
success, err := hybridLocker.Lock("my-lock-key")
```

**注意事项**
- `Unlock` 需要同一进程持有锁值；当本地没有锁值时会返回错误，以避免误删他人持有的锁。
- `HybridLocker` 在 Redis 失败时**不会**回退到本地锁。本地锁不提供跨实例互斥，在 Redis 故障期间降级恰恰是在最需要锁的时刻丢掉保证，而调用方从返回值上根本分辨不出来。`NewHybridLocker` 会返回 `lock.ErrRedisUnavailable`。
- 需要旧的降级行为时请显式使用 `NewHybridLockerWithLocalFallback`。降级生效期间跨实例互斥会丢失，只适用于允许出现第二个并发持有者的场景。通过回退持有的 key 在释放前不会再经 Redis 发放，其 unlock 也会被路由回本地锁。

#### Handle API：可续期的锁

`Lock`/`Unlock` 只用 key 标识一把锁，这也是 `Unlock` 必须检查本进程是否仍持有租约的
原因。`Acquire` 把 token 交给你，于是 `Release` 和 `Extend` 作用于**那一次具体的获取**：

```go
locker := lock.NewRedisLocker(client)

handle, err := locker.Acquire(ctx, "my-lock-key", 30*time.Second)
if err != nil {
    return err // 包含"已被持有"的情况——见下面的 errors.Is
}
defer handle.Release(ctx)

log.Printf("持有 %s 直到 %s", handle.Key(), handle.ExpiresAt())

// 工作时间可能超过租约时，请在租约到期前续期
if err := handle.Extend(ctx, 30*time.Second); err != nil {
    return err // 租约已经没了；请停止它所保护的工作
}
```

没有 `Extend` 时，TTL 固定为 `lock.DefaultLockTime`（15 秒），超过租约的工作会直接丢掉
锁，而且任何地方都不会报错。

#### 锁相关错误

| 哨兵错误 | 含义 |
|----------|------|
| `ErrRedisUnavailable` | Redis 故障。`NewHybridLocker` 返回它而不是降级为本地锁——请失败即关闭。 |
| `ErrLockExpired` | 在 `Unlock`/`Release` 之前租约已到期。没有发出删除，因此不会动到后来持有者的锁。 |
| `ErrLockNotHeld` | 本进程没有该 key 的锁值。 |
| `ErrLockValueMismatch` | 存储的值不是本进程的 token。 |
| `ErrLockValueType` | 存储的值类型不符合预期。 |
| `ErrLockTrackingLimit` | 进程级锁映射已满。请优先使用不需要映射的 `Acquire`/`Handle`。 |

请用 `errors.Is` 判断。

租约已到期的 `Unlock` 会报告 `ErrLockExpired` 且**不发出删除**，因此它不可能释放一个
后来的持有者已经获取的 key。被跟踪的条目会被周期性清扫，于是"获取后从不释放"的进程
（panic、提前返回、到期）不会让映射无界增长。

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

**注意事项**
- 限流与冷却检查使用 Redis Lua 脚本（`EVAL`）保证原子性，请确保 Redis 环境允许执行脚本。

`CheckLimit` 会校验 `limit`：`limit` 为 `0` 时会被拒绝，而不再被当成"还没有计数器"
——后者此前会放过每个窗口的第一个请求，于是"什么都不允许"反而让流量通过了。

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

// 获取值。未命中是 ErrKeyNotFound，它同时也匹配 redis.Nil。
var retrievedUser User
if err := c.Get(ctx, "user:123", &retrievedUser); err != nil {
    if errors.Is(err, cache.ErrKeyNotFound) || errors.Is(err, redis.Nil) {
        // 没有缓存
    }
    return err
}

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

## 环境要求

- **Go 1.27+**（`go.mod` 声明 `go 1.27.0`）
- Redis 服务器（测试时可选，提供 Mock Redis）

## 测试覆盖率

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

## 示例

### 完整示例：带限流的缓存与锁

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

## 升级说明（v1.6.0）

**`HybridLocker` 不再降级为本地锁。** 这是部署中最可能显现出来的改动，而且是有意为之。

- **Redis 故障时返回 `ErrRedisUnavailable`，而不是给一把本地锁。**
  `HybridLocker.Lock` 此前尝试 Redis，遇到*任何*错误都落到 `LocalLocker`。本地锁不提供
  跨实例互斥，于是 Redis 故障期间每个实例都拿到自己的"锁"并认为自己持有该 key——而调用方
  看到的是 `(true, nil)`，与一把真正的分布式锁毫无区别。保证恰好在最需要它的时刻消失了。
  **请处理 `ErrRedisUnavailable` 并失败即关闭**，或者在确实可以接受第二个并发持有者的
  场景使用 `NewHybridLockerWithLocalFallback` 保留旧行为——缓存预热可以，支付绝对不行。
- **过期的持有者不能再释放别人的锁。** token 此前存在一个以 key 为键的进程级映射里，
  于是同一个 key 的第二次获取会覆盖第一个持有者的 token，而第一个持有者的 `Unlock` 会
  拿*第二个*持有者的 token 去做比较删除。现在映射里同时记录租约截止时间，超过租约的
  `Unlock` 会报告 `ErrLockExpired` 且不发出删除。**此前会"意外成功"的 `Unlock` 现在会
  返回错误**——而这才是正确答案。
- **锁映射会被清扫且有上限。** 获取后从不释放的进程此前会让它无界增长；现在满了会报告
  `ErrLockTrackingLimit`。
- **新增 `Acquire`/`Handle`**，而且它是更好的 API：它把 token 交给调用方，于是
  `Release` 和 `Extend` 作用于具体的那一次获取，完全不需要共享映射。`Extend` 也补上了
  缺失的续期路径——TTL 此前固定为 15 秒且无法刷新，超过租约的工作会静默丢掉锁。
- **缓存未命中携带 `ErrKeyNotFound`，并且匹配 `redis.Nil`。** `cache.Get` 此前返回普通
  `fmt.Errorf`，于是 `errors.Is(err, redis.Nil)` 为假，调用方只能匹配错误文本。原始
  消息被保留。
- **`ratelimit.CheckLimit` 会校验 `limit`。** `limit=0` 时 Lua 脚本会走"还没有计数器"
  的分支并放过每个窗口的第一个请求，于是"什么都不允许"反而让流量通过了。
- **License 徽章此前写的是 MIT。** `LICENSE` 文件是 Apache 2.0。
- **环境要求里写的是 Go 1.26**；`go.mod` 需要 `1.27.0`。

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

Apache License 2.0 —— 详见 [LICENSE](LICENSE)。
