package client

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/soulteary/redis-kit/testutil"
)

func TestCheckHealth(t *testing.T) {
	t.Run("healthy status", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

		ctx := context.Background()
		status := CheckHealth(ctx, client)

		if !status.Healthy {
			t.Error("CheckHealth() healthy = false, want true")
		}
		if status.Error != nil {
			t.Errorf("CheckHealth() error = %v, want nil", status.Error)
		}
		if status.Latency <= 0 {
			t.Error("CheckHealth() latency should be positive")
		}
		if status.Timestamp.IsZero() {
			t.Error("CheckHealth() timestamp should be set")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		ctx := context.Background()
		status := CheckHealth(ctx, nil)

		if status.Healthy {
			t.Error("CheckHealth() with nil client healthy = true, want false")
		}
		if status.Error == nil {
			t.Error("CheckHealth() with nil client error = nil, want error")
		}
		if status.Error.Error() != "redis client is nil" {
			t.Errorf("CheckHealth() error = %q, want %q", status.Error.Error(), "redis client is nil")
		}
		if status.Timestamp.IsZero() {
			t.Error("CheckHealth() timestamp should be set even on error")
		}
	})

	t.Run("ping failure", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		})
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		status := CheckHealth(ctx, client)

		if status.Healthy {
			t.Error("CheckHealth() with invalid client healthy = true, want false")
		}
		if status.Error == nil {
			t.Error("CheckHealth() with invalid client error = nil, want error")
		}
		if status.Latency <= 0 {
			t.Error("CheckHealth() latency should be measured even on failure")
		}
		if status.Timestamp.IsZero() {
			t.Error("CheckHealth() timestamp should be set even on error")
		}
	})

	t.Run("latency measurement", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

		ctx := context.Background()
		status := CheckHealth(ctx, client)

		// Latency should be reasonable (less than 1 second for mock)
		if status.Latency > time.Second {
			t.Errorf("CheckHealth() latency = %v, should be less than 1s", status.Latency)
		}
		if status.Latency <= 0 {
			t.Error("CheckHealth() latency should be positive")
		}
	})

	t.Run("timestamp recording", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

		before := time.Now()
		ctx := context.Background()
		status := CheckHealth(ctx, client)
		after := time.Now()

		if status.Timestamp.Before(before) || status.Timestamp.After(after) {
			t.Errorf("CheckHealth() timestamp = %v, should be between %v and %v",
				status.Timestamp, before, after)
		}
	})
}
