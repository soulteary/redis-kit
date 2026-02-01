package client

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
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

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}

// NewClientWithDefaults creates a new Redis client with default configuration
func NewClientWithDefaults(addr string) (*redis.Client, error) {
	cfg := DefaultConfig().WithAddr(addr)
	return NewClient(cfg)
}

// Ping tests the connection to Redis
func Ping(ctx context.Context, client *redis.Client) error {
	if client == nil {
		return fmt.Errorf("redis client is nil")
	}

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	return nil
}

// Close closes the Redis client connection
func Close(client *redis.Client) error {
	if client == nil {
		return nil
	}
	return client.Close()
}

// HealthCheck performs a health check on the Redis connection
// Returns true if healthy, false otherwise
func HealthCheck(ctx context.Context, client *redis.Client) bool {
	if client == nil {
		return false
	}

	// Use a short timeout for health checks
	healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return client.Ping(healthCtx).Err() == nil
}
