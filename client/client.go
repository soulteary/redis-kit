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
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis address is required")
	}

	opts := &redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxRetries:   cfg.MaxRetries,
		PoolTimeout:  cfg.PoolTimeout,
	}
	if cfg.Dialer != nil {
		opts.Dialer = cfg.Dialer
	}

	client := redis.NewClient(opts)

	if err := verifyConnection(client, cfg); err != nil {
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
// [redis.UniversalOptions.Simple], so that NewClient keeps producing exactly
// the options it always did.
func NewUniversalClient(cfg Config) (redis.UniversalClient, error) {
	addrs := cfg.Addrs
	if len(addrs) == 0 {
		if cfg.Addr == "" {
			return nil, fmt.Errorf("redis address is required")
		}
		addrs = []string{cfg.Addr}
	}

	opts := &redis.UniversalOptions{
		Addrs:            addrs,
		MasterName:       cfg.MasterName,
		Password:         cfg.Password,
		SentinelUsername: cfg.SentinelUsername,
		SentinelPassword: cfg.SentinelPassword,
		DB:               cfg.DB,
		PoolSize:         cfg.PoolSize,
		MinIdleConns:     cfg.MinIdleConns,
		DialTimeout:      cfg.DialTimeout,
		ReadTimeout:      cfg.ReadTimeout,
		WriteTimeout:     cfg.WriteTimeout,
		MaxRetries:       cfg.MaxRetries,
		PoolTimeout:      cfg.PoolTimeout,
	}
	if cfg.Dialer != nil {
		opts.Dialer = cfg.Dialer
	}

	client := redis.NewUniversalClient(opts)

	if err := verifyConnection(client, cfg); err != nil {
		return nil, err
	}

	return client, nil
}

// verifyConnection pings a freshly built client and closes it if the server
// does not answer, so a constructor never hands back a client that was already
// known to be unreachable.
func verifyConnection(client redis.UniversalClient, cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout(cfg))
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
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
