package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultKeyPrefix is the default prefix for rate limit keys
	DefaultKeyPrefix = "ratelimit:"
	// DefaultCooldownPrefix is the default prefix for cooldown keys
	DefaultCooldownPrefix = "ratelimit:cooldown:"
)

const rateLimitScript = `
-- redis-kit:ratelimit
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local current = redis.call("get", key)
if not current then
	redis.call("set", key, 1, "px", window)
	return {1, limit - 1, window}
end
current = tonumber(current)
if current >= limit then
	local ttl = redis.call("pttl", key)
	return {0, 0, ttl}
end
current = redis.call("incr", key)
local ttl = redis.call("pttl", key)
if ttl < 0 then
	redis.call("pexpire", key, window)
	ttl = window
end
local remaining = limit - current
if remaining < 0 then
	remaining = 0
end
return {1, remaining, ttl}
`

const cooldownScript = `
-- redis-kit:cooldown
local key = KEYS[1]
local cooldown = tonumber(ARGV[1])
local res = redis.call("set", key, "1", "px", cooldown, "nx")
if res then
	return {1, cooldown}
end
local ttl = redis.call("pttl", key)
return {0, ttl}
`

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

	windowMs := window.Milliseconds()
	if windowMs <= 0 {
		return false, 0, time.Time{}, fmt.Errorf("window must be positive")
	}

	redisKey := r.keyPrefix + key

	result, err := r.client.Eval(ctx, rateLimitScript, []string{redisKey}, limit, windowMs).Result()
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to apply rate limit: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 3 {
		return false, 0, time.Time{}, fmt.Errorf("unexpected rate limit response")
	}

	allowedInt, ok := toInt64(values[0])
	if !ok {
		return false, 0, time.Time{}, fmt.Errorf("invalid rate limit allowed value")
	}
	remainingInt, ok := toInt64(values[1])
	if !ok {
		return false, 0, time.Time{}, fmt.Errorf("invalid rate limit remaining value")
	}
	ttlMs, ok := toInt64(values[2])
	if !ok {
		return false, 0, time.Time{}, fmt.Errorf("invalid rate limit ttl value")
	}

	if ttlMs < 0 {
		ttlMs = 0
	}
	resetTime := time.Now().Add(time.Duration(ttlMs) * time.Millisecond)

	return allowedInt == 1, int(remainingInt), resetTime, nil
}

// CheckCooldown checks if resend is allowed (cooldown period)
// Returns (allowed, resetTime, error)
func (r *RateLimiter) CheckCooldown(ctx context.Context, key string, cooldown time.Duration) (bool, time.Time, error) {
	if r.client == nil {
		return false, time.Time{}, fmt.Errorf("redis client is nil")
	}

	cooldownMs := cooldown.Milliseconds()
	if cooldownMs <= 0 {
		return false, time.Time{}, fmt.Errorf("cooldown must be positive")
	}

	redisKey := r.cooldownPrefix + key

	result, err := r.client.Eval(ctx, cooldownScript, []string{redisKey}, cooldownMs).Result()
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to apply cooldown: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		return false, time.Time{}, fmt.Errorf("unexpected cooldown response")
	}

	allowedInt, ok := toInt64(values[0])
	if !ok {
		return false, time.Time{}, fmt.Errorf("invalid cooldown allowed value")
	}
	ttlMs, ok := toInt64(values[1])
	if !ok {
		return false, time.Time{}, fmt.Errorf("invalid cooldown ttl value")
	}
	if ttlMs < 0 {
		ttlMs = 0
	}
	resetTime := time.Now().Add(time.Duration(ttlMs) * time.Millisecond)

	return allowedInt == 1, resetTime, nil
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

func toInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
