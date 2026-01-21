package client

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// HealthStatus represents the health status of a Redis connection
type HealthStatus struct {
	Healthy   bool
	Latency   time.Duration
	Error     error
	Timestamp time.Time
}

// CheckHealth performs a comprehensive health check
func CheckHealth(ctx context.Context, client *redis.Client) HealthStatus {
	status := HealthStatus{
		Timestamp: time.Now(),
	}

	if client == nil {
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
