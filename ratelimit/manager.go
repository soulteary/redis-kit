package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultKeyPrefix is the default prefix for rate limit keys
	DefaultKeyPrefix = "ratelimit:"
	// DefaultCooldownPrefix is the default prefix for cooldown keys
	DefaultCooldownPrefix = "ratelimit:cooldown:"
)

// RateLimiter provides rate limiting functionality using Redis
type RateLimiter struct {
	client         *redis.Client
	keyPrefix      string
	cooldownPrefix string
}

// NewRateLimiter creates a new rate limiter with default prefixes
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return NewRateLimiterWithPrefixes(client, DefaultKeyPrefix, DefaultCooldownPrefix)
}

// NewRateLimiterWithPrefixes creates a new rate limiter with custom prefixes
func NewRateLimiterWithPrefixes(client *redis.Client, keyPrefix, cooldownPrefix string) *RateLimiter {
	return &RateLimiter{
		client:         client,
		keyPrefix:      keyPrefix,
		cooldownPrefix: cooldownPrefix,
	}
}

// CheckLimit checks if a request should be rate limited
// Returns (allowed, remaining, resetTime, error)
func (r *RateLimiter) CheckLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	if r.client == nil {
		return false, 0, time.Time{}, fmt.Errorf("redis client is nil")
	}

	redisKey := r.keyPrefix + key

	// Get current count
	count, err := r.client.Get(ctx, redisKey).Int()
	if err == redis.Nil {
		// First request, set count to 1
		if err := r.client.Set(ctx, redisKey, 1, window).Err(); err != nil {
			return false, 0, time.Time{}, fmt.Errorf("failed to set rate limit: %w", err)
		}
		remaining := limit - 1
		resetTime := time.Now().Add(window)
		return true, remaining, resetTime, nil
	}
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to get rate limit: %w", err)
	}

	// Check if limit exceeded
	if count >= limit {
		ttl := r.client.TTL(ctx, redisKey).Val()
		resetTime := time.Now().Add(ttl)
		return false, 0, resetTime, nil
	}

	// Increment count
	newCount, err := r.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to increment rate limit: %w", err)
	}

	// Set expiration if this is the first increment (key was just created)
	// When key already exists, expiration is already set, so we only need to set it for new keys
	if newCount == 1 {
		if err := r.client.Expire(ctx, redisKey, window).Err(); err != nil {
			// Log warning but don't fail - expiration might already be set
			_ = err
		}
	} else {
		// Ensure expiration is set even if key existed (defensive programming)
		ttl := r.client.TTL(ctx, redisKey).Val()
		if ttl <= 0 {
			// Key exists but has no expiration, set it
			if err := r.client.Expire(ctx, redisKey, window).Err(); err != nil {
				// Log warning but don't fail
				_ = err
			}
		}
	}

	remaining := limit - int(newCount)
	if remaining < 0 {
		remaining = 0
	}

	ttl := r.client.TTL(ctx, redisKey).Val()
	resetTime := time.Now().Add(ttl)

	return true, remaining, resetTime, nil
}

// CheckCooldown checks if resend is allowed (cooldown period)
// Returns (allowed, resetTime, error)
func (r *RateLimiter) CheckCooldown(ctx context.Context, key string, cooldown time.Duration) (bool, time.Time, error) {
	if r.client == nil {
		return false, time.Time{}, fmt.Errorf("redis client is nil")
	}

	redisKey := r.cooldownPrefix + key

	exists, err := r.client.Exists(ctx, redisKey).Result()
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to check cooldown: %w", err)
	}

	if exists > 0 {
		ttl := r.client.TTL(ctx, redisKey).Val()
		resetTime := time.Now().Add(ttl)
		return false, resetTime, nil
	}

	// Set cooldown
	if err := r.client.Set(ctx, redisKey, "1", cooldown).Err(); err != nil {
		return false, time.Time{}, fmt.Errorf("failed to set cooldown: %w", err)
	}

	resetTime := time.Now().Add(cooldown)
	return true, resetTime, nil
}

// CheckUserLimit checks rate limit for a user
func (r *RateLimiter) CheckUserLimit(ctx context.Context, userID string, limit int, window time.Duration) (bool, int, time.Time, error) {
	key := fmt.Sprintf("user:%s", userID)
	return r.CheckLimit(ctx, key, limit, window)
}

// CheckIPLimit checks rate limit for an IP address
func (r *RateLimiter) CheckIPLimit(ctx context.Context, ip string, limit int, window time.Duration) (bool, int, time.Time, error) {
	key := fmt.Sprintf("ip:%s", ip)
	return r.CheckLimit(ctx, key, limit, window)
}

// CheckDestinationLimit checks rate limit for a destination (phone/email)
func (r *RateLimiter) CheckDestinationLimit(ctx context.Context, destination string, limit int, window time.Duration) (bool, int, time.Time, error) {
	key := fmt.Sprintf("dest:%s", destination)
	return r.CheckLimit(ctx, key, limit, window)
}
