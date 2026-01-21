package utils

import (
	"context"
	"time"
)

const (
	// DefaultOperationTimeout is the default timeout for Redis operations (5 seconds)
	DefaultOperationTimeout = 5 * time.Second
)

// WithTimeout creates a context with the given timeout
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeout)
}

// WithDefaultTimeout creates a context with the default operation timeout
func WithDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(ctx, DefaultOperationTimeout)
}
