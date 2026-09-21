package client

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soulteary/redis-kit/internal/nilcheck"
)

// HealthStatus represents the health status of a Redis connection
type HealthStatus struct {
	Healthy   bool
	Latency   time.Duration
	Error     error
	Timestamp time.Time
}

// CheckHealth performs a comprehensive health check.
//
// client is a redis.UniversalClient, so every go-redis client shape is
// accepted. A nil client -- including a typed nil -- is reported as an
// unhealthy status rather than dereferenced.
func CheckHealth(ctx context.Context, client redis.UniversalClient) HealthStatus {
	status := HealthStatus{
		Timestamp: time.Now(),
	}

	if nilcheck.IsNil(client) {
		status.Error = fmt.Errorf("redis client is nil")
		return status
	}

	// Measure latency
	start := time.Now()
	healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := client.Ping(healthCtx).Err()
	status.Latency = time.Since(start)

	if err != nil {
		status.Error = err
		status.Healthy = false
	} else {
		status.Healthy = true
	}

	return status
}
