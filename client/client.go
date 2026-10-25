package client

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soulteary/redis-kit/internal/nilcheck"
)

// NewClient creates a new Redis client with the given configuration
func NewClient(cfg Config) (*redis.Client, error) {
	return NewClientContext(context.Background(), cfg)
}

// NewClientContext creates a single-node client and verifies it with PING.
// ctx cancellation and the smaller of its deadline and DialTimeout bound the
// entire startup, including dialing, TLS, authentication and PING. Once this
// function returns, ctx does not control the lifetime of the client.
func NewClientContext(ctx context.Context, cfg Config) (*redis.Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis address is required")
	}
	ctx, cancel := context.WithTimeout(ctx, dialTimeout(cfg))
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	dialer, detach := startupDialer(ctx, cfg)
	defer detach()

	opts := &redis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
		// Keep pipelines in the same pool so PoolSize remains the connection ceiling.
		PipelinePoolSize:      -1,
		MinIdleConns:          cfg.MinIdleConns,
		DialTimeout:           cfg.DialTimeout,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		MaxRetries:            cfg.MaxRetries,
		PoolTimeout:           cfg.PoolTimeout,
		TLSConfig:             cloneTLSConfig(cfg.TLSConfig),
		ContextTimeoutEnabled: cfg.ContextTimeoutEnabled,
		Dialer:                dialer,
	}

	client := redis.NewClient(opts)

	if err := verifyConnection(ctx, client); err != nil {
		return nil, err
	}

	return client, nil
}

// NewUniversalClient creates whichever go-redis client the configuration
// describes: a Sentinel-backed failover client when MasterName is set, a
// cluster client when Addrs holds more than one address, and a single-node
// client otherwise. See [redis.NewUniversalClient] for the exact rules.
//
// This is the constructor to use with the rest of redis-kit, whose
// constructors all take a redis.UniversalClient. [NewClient] remains the way
// to get a concrete *redis.Client when you need one -- to reach Options() or
// to pass it somewhere that still insists on the concrete type.
//
// The field mapping is written out here rather than routed through
// [redis.UniversalOptions.Simple], keeping the standalone field mapping explicit.
func NewUniversalClient(cfg Config) (redis.UniversalClient, error) {
	return NewUniversalClientContext(context.Background(), cfg)
}

// NewUniversalClientContext selects the same client shape as
// [NewUniversalClient], with the startup context semantics of [NewClientContext].
func NewUniversalClientContext(ctx context.Context, cfg Config) (redis.UniversalClient, error) {
	addrs := cfg.Addrs
	if len(addrs) == 0 {
		if cfg.Addr == "" {
			return nil, fmt.Errorf("redis address is required")
		}
		addrs = []string{cfg.Addr}
	}
	ctx, cancel := context.WithTimeout(ctx, dialTimeout(cfg))
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	dialer, detach := startupDialer(ctx, cfg)
	defer detach()

	opts := &redis.UniversalOptions{
		Addrs:            addrs,
		MasterName:       cfg.MasterName,
		Username:         cfg.Username,
		Password:         cfg.Password,
		SentinelUsername: cfg.SentinelUsername,
		SentinelPassword: cfg.SentinelPassword,
		DB:               cfg.DB,
		PoolSize:         cfg.PoolSize,
		// Apply the same ceiling to standalone, failover and cluster node pipelines.
		PipelinePoolSize:      -1,
		MinIdleConns:          cfg.MinIdleConns,
		DialTimeout:           cfg.DialTimeout,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		MaxRetries:            cfg.MaxRetries,
		PoolTimeout:           cfg.PoolTimeout,
		TLSConfig:             cloneTLSConfig(cfg.TLSConfig),
		ContextTimeoutEnabled: cfg.ContextTimeoutEnabled,
		Dialer:                dialer,
	}

	client := redis.NewUniversalClient(opts)

	if err := verifyConnection(ctx, client); err != nil {
		return nil, err
	}

	return client, nil
}

// verifyConnection pings a freshly built client and closes it if the server
// does not answer, so a constructor never hands back a client that was already
// known to be unreachable.
func verifyConnection(ctx context.Context, client redis.UniversalClient) error {
	// Closing the client interrupts authentication/PING even when command
	// context deadlines are disabled. Do not leave a cancellation callback
	// attached to a successfully returned client.
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	err := client.Ping(ctx).Err()
	stopped := stop()
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if deadline, ok := ctx.Deadline(); err != nil && ok && !time.Now().Before(deadline) {
		// Socket deadlines can fire just before the context's timer goroutine.
		// Report the constructor's expired budget consistently in that race.
		err = context.DeadlineExceeded
	}
	if !stopped || err != nil {
		_ = client.Close()
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	return nil
}

// dialTimeout bounds the connection test.
//
// A hand-built Config{Addr: ...} leaves DialTimeout at zero, and a zero
// timeout produces a context that has already expired -- so every such call
// failed with "context deadline exceeded" without a packet being sent. Fall
// back to the documented default instead.
func dialTimeout(cfg Config) time.Duration {
	if cfg.DialTimeout <= 0 {
		return DefaultConfig().DialTimeout
	}
	return cfg.DialTimeout
}

// NewClientWithDefaults creates a new Redis client with default configuration
func NewClientWithDefaults(addr string) (*redis.Client, error) {
	cfg := DefaultConfig().WithAddr(addr)
	return NewClient(cfg)
}

// Ping tests the connection to Redis.
//
// client is a redis.UniversalClient, so a standalone client, a cluster, a ring
// and a Sentinel-backed failover client are all accepted. A nil client --
// including a typed nil such as an unassigned *redis.Client field -- is
// reported as an error rather than dereferenced.
func Ping(ctx context.Context, client redis.UniversalClient) error {
	if nilcheck.IsNil(client) {
		return fmt.Errorf("redis client is nil")
	}

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	return nil
}

// Close closes the Redis client connection. A nil client is a no-op.
func Close(client redis.UniversalClient) error {
	if nilcheck.IsNil(client) {
		return nil
	}
	return client.Close()
}

// HealthCheck performs a health check on the Redis connection
// Returns true if healthy, false otherwise
func HealthCheck(ctx context.Context, client redis.UniversalClient) bool {
	if nilcheck.IsNil(client) {
		return false
	}

	// Use a short timeout for health checks
	healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return client.Ping(healthCtx).Err() == nil
}
