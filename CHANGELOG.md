# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Go encodes the major version in the import path, so a major release would also
change the module path. It has not happened yet: the module path is still
`github.com/soulteary/redis-kit`.

## [Unreleased]

## [1.7.0] — 2026-09-21

### Changed

- **Every client parameter is now a `redis.UniversalClient` instead of a
  `*redis.Client`.** A Redis Cluster, a Sentinel-backed failover setup and a
  Ring could not use this library at all — not awkwardly, not with a wrapper:
  the compiler refused the call. A consumer holding a `redis.UniversalClient`
  and calling `cache.NewCache`, `lock.NewRedisLocker` and
  `ratelimit.NewRateLimiter` failed to build with three errors of the form
  `cannot use rdb (variable of interface type redis.UniversalClient) as
  *redis.Client value in argument to cache.NewCache: need type assertion`. It
  now compiles.

  Eleven exported functions changed parameter type:

  | Package | Functions |
  |---|---|
  | `cache` | `NewCache` |
  | `client` | `Ping`, `Close`, `HealthCheck`, `CheckHealth` |
  | `lock` | `NewRedisLocker`, `NewRedisLockerWithLockTime`, `NewHybridLocker`, `NewHybridLockerWithLocalFallback` |
  | `ratelimit` | `NewRateLimiter`, `NewRateLimiterWithPrefixes` |

  **Existing code keeps compiling.** `*redis.Client` satisfies
  `redis.UniversalClient`, so every call that worked before still works and no
  import changes. The one shape that breaks is taking the function as a value
  at its old concrete type — `var f func(*redis.Client, string) *cache.RedisCache = cache.NewCache`.
  That is why this is a minor release and the module path is unchanged: there
  is no reason to spend a major version, and no reason to make every user edit
  an import path for a change that costs them nothing.

  Measured for a program importing `cache`, `client`, `lock` and `ratelimit`
  and passing a `*redis.Client`, built `-trimpath` against v1.6.0 and against
  v1.7.0:

  | | v1.6.0 | v1.7.0 |
  |---|---|---|
  | binary | 10,631,474 B | 10,657,708 B (+26,234, +0.25%) |
  | linked packages | 202 | 203 (`internal/nilcheck`) |
  | modules in `go.mod` | 13 | 13 |
  | lines in `go.sum` | 22 | 22 |

  So the dependency footprint is unchanged — no new module, nothing added to
  `go.sum`, no new minimum version pushed onto consumers through MVS — and the
  binary grows by a quarter of a percent.

- **A zero `DialTimeout` no longer fails every connection.** `NewClient`
  handed `cfg.DialTimeout` straight to `context.WithTimeout`, so a hand-built
  `Config{Addr: "..."}` produced an already-expired context and failed with
  `failed to connect to Redis: context deadline exceeded` without a packet
  being sent. A non-positive `DialTimeout` now falls back to the documented
  5s default. `DefaultConfig()` set it, so only hand-built configs were
  affected — and for those the call could not previously succeed at all.

### Added

- **`client.NewUniversalClient`**, which builds whichever client the
  configuration describes: a Sentinel-backed failover client when
  `MasterName` is set, a cluster client when `Addrs` holds more than one
  address, and a single-node client otherwise. Accepting every client shape
  is only half of the problem; a cluster user who still has to drop down to
  go-redis to construct one has not been served. `NewClient` is unchanged and
  still returns a concrete `*redis.Client`.
- **`Config.Addrs`, `Config.MasterName`, `Config.SentinelUsername` and
  `Config.SentinelPassword`**, with `WithAddrs`, `WithMasterName` and
  `WithSentinelAuth`. The Sentinel credentials are deliberately separate from
  `Password`: they authenticate to the Sentinel nodes, not to the Redis server
  behind them, and the two routinely differ. Only `NewUniversalClient` reads
  these fields.
- **Runnable examples** for `cache`, `client`, `lock` and `ratelimit` that
  `go test` verifies, so they cannot drift from the API. The cluster and
  Sentinel ones are compiled but not run, having nothing to connect to.
- **A package doc in `doc.go`** describing the layout, which client type to
  pass, and what a cluster does *not* buy you for locking.
- **`.github/workflows/release.yml`.** Ten tags exist with nothing having
  checked any of them. It runs the CI gate against the tagged commit plus the
  two checks that only matter at tag time: the module path must carry the
  tag's major version (v0 and v1 taking no suffix), and both READMEs' `go get`
  line must name that same path.
- **`CHANGELOG.md`** — this file.

### Fixed

- **A nil client is reported rather than dereferenced, including a typed
  nil.** This is the hole that taking an interface reopens, and it is not
  hypothetical: an unassigned `*redis.Client` field assigned into a
  `redis.UniversalClient` is a *non-nil* interface holding a nil pointer, so
  the previous `client == nil` guards waved it straight through to a nil
  dereference. Verified: reverting the guard in `cache` alone turns
  `TestNewCacheAcceptsEveryClientShape` into
  `SIGSEGV: segmentation violation`. Every nil check now looks inside the
  interface (`internal/nilcheck`) and returns the same `redis client is nil`
  error text as before.

  `NewHybridLocker` is the sharpest case. It chose its backend with
  `client != nil`, which is *true* for a typed nil — so what used to be a
  working process-local lock would have become a panic on the first `Lock`.
  It now treats a typed nil as "no Redis", exactly as it treats an untyped
  one.

## [1.6.0] — 2026-09-12

Locking correctness, and the end of silent degradation. See the "Upgrade
Notes (v1.6.0)" section of the README for the full account.

### Changed — BREAKING

- `HybridLocker` returns `ErrRedisUnavailable` on a Redis failure instead of
  silently degrading to a process-local lock, which provided no exclusion
  between instances. `NewHybridLockerWithLocalFallback` opts back in.
- An `Unlock` past its lease reports `ErrLockExpired` without issuing the
  delete, so a stale holder can no longer release a later holder's lock.

### Added

- `RedisLocker.Acquire` and `Handle`, which carry the acquisition's token so
  `Release` and `Extend` act on a specific acquisition. `Extend` fills in the
  renewal path that did not exist.
- `cache.ErrKeyNotFound`, which also matches `redis.Nil`.

### Fixed

- `ratelimit.CheckLimit` with `limit=0` admitted the first request of every
  window instead of blocking.
- The lock tracking map is bounded, reporting `ErrLockTrackingLimit` when full.
