package client

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/soulteary/redis-kit/testutil"
)

func TestNewClient(t *testing.T) {
	t.Run("successful creation with mock", func(t *testing.T) {
		mockClient, _ := testutil.NewMockRedisClient()
		defer mockClient.Close()

		cfg := DefaultConfig().WithAddr("mock")
		// We can't use NewClient with mock because it tries to ping
		// So we'll test the validation logic separately
		if cfg.Addr == "" {
			t.Error("config should have address")
		}
	})

	t.Run("empty address error", func(t *testing.T) {
		cfg := DefaultConfig().WithAddr("")
		_, err := NewClient(cfg)
		if err == nil {
			t.Error("NewClient() with empty address should return error")
		}
		if err.Error() != "redis address is required" {
			t.Errorf("NewClient() error = %q, want %q", err.Error(), "redis address is required")
		}
	})

	t.Run("connection failure", func(t *testing.T) {
		cfg := DefaultConfig().WithAddr("invalid:6379").WithDialTimeout(100 * time.Millisecond)
		_, err := NewClient(cfg)
		if err == nil {
			t.Error("NewClient() with invalid address should return error")
		}
	})
}

func TestNewClientWithDefaults(t *testing.T) {
	t.Run("creates client with default config", func(t *testing.T) {
		// This will fail without real Redis, but we can test the function exists
		_, err := NewClientWithDefaults("invalid:6379")
		if err == nil {
			t.Error("NewClientWithDefaults() with invalid address should return error")
		}
	})
}

func TestPing(t *testing.T) {
	t.Run("successful ping", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

		ctx := context.Background()
		err := Ping(ctx, client)
		if err != nil {
			t.Errorf("Ping() error = %v, want nil", err)
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		ctx := context.Background()
		err := Ping(ctx, nil)
		if err == nil {
			t.Error("Ping() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Ping() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("ping failure", func(t *testing.T) {
		// Create a client that will fail
		client := redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		})
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := Ping(ctx, client)
		if err == nil {
			t.Error("Ping() with invalid client should return error")
		}
	})
}

func TestClose(t *testing.T) {
	t.Run("successful close", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		err := Close(client)
		if err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})

	t.Run("nil client", func(t *testing.T) {
		err := Close(nil)
		if err != nil {
			t.Errorf("Close() with nil client should return nil, got %v", err)
		}
	})
}

func TestHealthCheck(t *testing.T) {
	t.Run("healthy client", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

		ctx := context.Background()
		healthy := HealthCheck(ctx, client)
		if !healthy {
			t.Error("HealthCheck() = false, want true")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		ctx := context.Background()
		healthy := HealthCheck(ctx, nil)
		if healthy {
			t.Error("HealthCheck() with nil client = true, want false")
		}
	})

	t.Run("unhealthy client", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		})
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		healthy := HealthCheck(ctx, client)
		if healthy {
			t.Error("HealthCheck() with invalid client = true, want false")
		}
	})

	t.Run("timeout scenario", func(t *testing.T) {
		// Create a client that will timeout
		client := redis.NewClient(&redis.Options{
			Addr:         "192.0.2.1:6379", // Test IP that should not respond
			DialTimeout:  10 * time.Millisecond,
			ReadTimeout:  10 * time.Millisecond,
			WriteTimeout: 10 * time.Millisecond,
		})
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		healthy := HealthCheck(ctx, client)
		if healthy {
			t.Error("HealthCheck() with timeout = true, want false")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Ping with cancelled context
		err := Ping(ctx, client)
		if err == nil {
			t.Error("Ping() with cancelled context should return error")
		}
	})
}
