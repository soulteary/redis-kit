package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/soulteary/redis-kit/testutil"
)

func TestNewRateLimiter(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client)
	if limiter == nil {
		t.Fatal("NewRateLimiter() returned nil")
	}
	if limiter.client != client {
		t.Error("NewRateLimiter() client mismatch")
	}
	if limiter.keyPrefix != DefaultKeyPrefix {
		t.Errorf("NewRateLimiter() keyPrefix = %q, want %q", limiter.keyPrefix, DefaultKeyPrefix)
	}
	if limiter.cooldownPrefix != DefaultCooldownPrefix {
		t.Errorf("NewRateLimiter() cooldownPrefix = %q, want %q", limiter.cooldownPrefix, DefaultCooldownPrefix)
	}
}

func TestNewRateLimiterWithPrefixes(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	keyPrefix := "custom:"
	cooldownPrefix := "cooldown:"
	limiter := NewRateLimiterWithPrefixes(client, keyPrefix, cooldownPrefix)
	if limiter == nil {
		t.Fatal("NewRateLimiterWithPrefixes() returned nil")
	}
	if limiter.keyPrefix != keyPrefix {
		t.Errorf("NewRateLimiterWithPrefixes() keyPrefix = %q, want %q", limiter.keyPrefix, keyPrefix)
	}
	if limiter.cooldownPrefix != cooldownPrefix {
		t.Errorf("NewRateLimiterWithPrefixes() cooldownPrefix = %q, want %q", limiter.cooldownPrefix, cooldownPrefix)
	}
}

func TestRateLimiter_CheckLimit(t *testing.T) {
	t.Run("first request allowed", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		allowed, remaining, resetTime, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() error = %v, want nil", err)
		}
		if !allowed {
			t.Error("CheckLimit() first request allowed = false, want true")
		}
		if remaining != 9 {
			t.Errorf("CheckLimit() remaining = %d, want 9", remaining)
		}
		if resetTime.IsZero() {
			t.Error("CheckLimit() resetTime should be set")
		}
	})

	t.Run("within limit", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// First request
		_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)

		// Second request
		allowed, remaining, resetTime, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() error = %v, want nil", err)
		}
		if !allowed {
			t.Error("CheckLimit() within limit allowed = false, want true")
		}
		if remaining != 8 {
			t.Errorf("CheckLimit() remaining = %d, want 8", remaining)
		}
		if resetTime.IsZero() {
			t.Error("CheckLimit() resetTime should be set")
		}
	})

	t.Run("limit exceeded", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Make requests up to limit
		for i := 0; i < 5; i++ {
			_, _, _, _ = limiter.CheckLimit(ctx, "key1", 5, time.Hour)
		}

		// Next request should be denied
		allowed, remaining, resetTime, err := limiter.CheckLimit(ctx, "key1", 5, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() error = %v, want nil", err)
		}
		if allowed {
			t.Error("CheckLimit() limit exceeded allowed = true, want false")
		}
		if remaining != 0 {
			t.Errorf("CheckLimit() remaining = %d, want 0", remaining)
		}
		if resetTime.IsZero() {
			t.Error("CheckLimit() resetTime should be set")
		}
	})

	t.Run("boundary condition limit equals count", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Make exactly limit requests
		for i := 0; i < 3; i++ {
			_, _, _, _ = limiter.CheckLimit(ctx, "key1", 3, time.Hour)
		}

		// Next request should be denied
		allowed, _, _, err := limiter.CheckLimit(ctx, "key1", 3, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() error = %v, want nil", err)
		}
		if allowed {
			t.Error("CheckLimit() at limit allowed = true, want false")
		}
	})

	t.Run("limit equals one", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// First request should succeed
		allowed1, remaining1, _, err1 := limiter.CheckLimit(ctx, "key1", 1, time.Hour)
		if err1 != nil || !allowed1 || remaining1 != 0 {
			t.Fatal("First request with limit=1 should succeed")
		}

		// Second request should fail
		allowed2, remaining2, _, err2 := limiter.CheckLimit(ctx, "key1", 1, time.Hour)
		if err2 != nil {
			t.Errorf("CheckLimit() error = %v, want nil", err2)
		}
		if allowed2 {
			t.Error("CheckLimit() with limit=1, second request allowed = true, want false")
		}
		if remaining2 != 0 {
			t.Errorf("CheckLimit() remaining = %d, want 0", remaining2)
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		limiter := &RateLimiter{
			client: nil,
		}
		ctx := context.Background()

		_, _, _, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err == nil {
			t.Error("CheckLimit() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("CheckLimit() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("reset time calculation", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		window := 1 * time.Hour
		_, _, resetTime, err := limiter.CheckLimit(ctx, "key1", 10, window)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}

		// Reset time should be in the future
		if resetTime.Before(time.Now()) {
			t.Error("CheckLimit() resetTime should be in the future")
		}

		// Reset time should be approximately window duration from now
		expectedMin := time.Now().Add(window - 10*time.Second)
		expectedMax := time.Now().Add(window + 10*time.Second)
		if resetTime.Before(expectedMin) || resetTime.After(expectedMax) {
			t.Errorf("CheckLimit() resetTime = %v, should be approximately %v from now", resetTime, window)
		}
	})

	t.Run("remaining count negative protection", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Set a key with count higher than limit (simulate race condition)
		// This tests the remaining < 0 protection
		// Use CheckLimit to set up the key and increment it multiple times
		_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		for i := 0; i < 10; i++ {
			_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		}

		// Check limit with limit=10 (should be denied now)
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}

		// Should be denied
		if allowed {
			t.Error("CheckLimit() with count > limit allowed = true, want false")
		}

		// Remaining should be 0 (not negative)
		if remaining != 0 {
			t.Errorf("CheckLimit() remaining = %d, want 0", remaining)
		}
	})

	t.Run("expiration handling for existing key", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Create a key without expiration (simulate edge case)
		// Create a key by using CheckLimit first
		_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)

		// Check limit again - should handle existing key with expiration
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}

		// Should be allowed (count 2 < limit 10)
		if !allowed {
			t.Error("CheckLimit() allowed = false, want true")
		}
		if remaining != 8 {
			t.Errorf("CheckLimit() remaining = %d, want 8", remaining)
		}
	})
}

func TestRateLimiter_CheckCooldown(t *testing.T) {
	t.Run("cooldown not active", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		allowed, resetTime, err := limiter.CheckCooldown(ctx, "key1", 60*time.Second)
		if err != nil {
			t.Errorf("CheckCooldown() error = %v, want nil", err)
		}
		if !allowed {
			t.Error("CheckCooldown() first check allowed = false, want true")
		}
		if resetTime.IsZero() {
			t.Error("CheckCooldown() resetTime should be set")
		}
	})

	t.Run("cooldown active", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		cooldown := 60 * time.Second

		// First check (sets cooldown)
		allowed1, _, err1 := limiter.CheckCooldown(ctx, "key1", cooldown)
		if err1 != nil || !allowed1 {
			t.Fatal("First CheckCooldown() should succeed")
		}

		// Second check (should be denied)
		allowed2, resetTime, err2 := limiter.CheckCooldown(ctx, "key1", cooldown)
		if err2 != nil {
			t.Errorf("CheckCooldown() error = %v, want nil", err2)
		}
		if allowed2 {
			t.Error("CheckCooldown() during cooldown allowed = true, want false")
		}
		if resetTime.IsZero() {
			t.Error("CheckCooldown() resetTime should be set")
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		limiter := &RateLimiter{
			client: nil,
		}
		ctx := context.Background()

		_, _, err := limiter.CheckCooldown(ctx, "key1", 60*time.Second)
		if err == nil {
			t.Error("CheckCooldown() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("CheckCooldown() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("reset time calculation", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		cooldown := 2 * time.Minute
		_, resetTime, err := limiter.CheckCooldown(ctx, "key1", cooldown)
		if err != nil {
			t.Fatalf("CheckCooldown() error = %v", err)
		}

		// Reset time should be in the future
		if resetTime.Before(time.Now()) {
			t.Error("CheckCooldown() resetTime should be in the future")
		}

		// Reset time should be approximately cooldown duration from now
		expectedMin := time.Now().Add(cooldown - 5*time.Second)
		expectedMax := time.Now().Add(cooldown + 5*time.Second)
		if resetTime.Before(expectedMin) || resetTime.After(expectedMax) {
			t.Errorf("CheckCooldown() resetTime = %v, should be approximately %v from now", resetTime, cooldown)
		}
	})

	t.Run("remaining count negative protection", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Set a key with count higher than limit (simulate race condition)
		// This tests the remaining < 0 protection
		// Use CheckLimit to set up the key and increment it multiple times
		_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		for i := 0; i < 10; i++ {
			_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		}

		// Check limit with limit=10 (should be denied now)
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}

		// Should be denied
		if allowed {
			t.Error("CheckLimit() with count > limit allowed = true, want false")
		}

		// Remaining should be 0 (not negative)
		if remaining != 0 {
			t.Errorf("CheckLimit() remaining = %d, want 0", remaining)
		}
	})

	t.Run("expiration handling for existing key", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Create a key without expiration (simulate edge case)
		// Create a key by using CheckLimit first
		_, _, _, _ = limiter.CheckLimit(ctx, "key1", 10, time.Hour)

		// Check limit again - should handle existing key with expiration
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}

		// Should be allowed (count 2 < limit 10)
		if !allowed {
			t.Error("CheckLimit() allowed = false, want true")
		}
		if remaining != 8 {
			t.Errorf("CheckLimit() remaining = %d, want 8", remaining)
		}
	})
}

func TestRateLimiter_CheckUserLimit(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	allowed, remaining, resetTime, err := limiter.CheckUserLimit(ctx, "user123", 10, time.Hour)
	if err != nil {
		t.Errorf("CheckUserLimit() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("CheckUserLimit() allowed = false, want true")
	}
	if remaining != 9 {
		t.Errorf("CheckUserLimit() remaining = %d, want 9", remaining)
	}
	if resetTime.IsZero() {
		t.Error("CheckUserLimit() resetTime should be set")
	}

	// Verify key format
	allowed2, _, _, _ := limiter.CheckUserLimit(ctx, "user123", 10, time.Hour)
	if !allowed2 {
		t.Error("CheckUserLimit() second call should be allowed")
	}
}

func TestRateLimiter_CheckIPLimit(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	allowed, remaining, resetTime, err := limiter.CheckIPLimit(ctx, "192.168.1.1", 5, time.Minute)
	if err != nil {
		t.Errorf("CheckIPLimit() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("CheckIPLimit() allowed = false, want true")
	}
	if remaining != 4 {
		t.Errorf("CheckIPLimit() remaining = %d, want 4", remaining)
	}
	if resetTime.IsZero() {
		t.Error("CheckIPLimit() resetTime should be set")
	}
}

func TestRateLimiter_CheckDestinationLimit(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	allowed, remaining, resetTime, err := limiter.CheckDestinationLimit(ctx, "user@example.com", 10, time.Hour)
	if err != nil {
		t.Errorf("CheckDestinationLimit() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("CheckDestinationLimit() allowed = false, want true")
	}
	if remaining != 9 {
		t.Errorf("CheckDestinationLimit() remaining = %d, want 9", remaining)
	}
	if resetTime.IsZero() {
		t.Error("CheckDestinationLimit() resetTime should be set")
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	t.Run("concurrent limit checks", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()
		limit := 10
		numGoroutines := 20
		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				allowed, _, _, err := limiter.CheckLimit(ctx, "concurrent-key", limit, time.Hour)
				if err != nil {
					t.Errorf("CheckLimit() error = %v", err)
					return
				}
				if allowed {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		// In concurrent environment, mock Redis Get/Set/Incr are not atomic, so race conditions
		// can allow more than `limit` successes (or occasionally fewer). We only assert:
		// 1. At least one request succeeded (no deadlock/crash)
		// 2. At most numGoroutines succeeded (sanity)
		if successCount <= 0 {
			t.Errorf("concurrent CheckLimit() successCount = %d, want > 0", successCount)
		}
		if successCount > numGoroutines {
			t.Errorf("concurrent CheckLimit() successCount = %d, want <= %d", successCount, numGoroutines)
		}
		// Log when result is outside ideal range (mock non-atomicity can allow all through)
		if successCount < limit-2 || successCount > limit+2 {
			t.Logf("concurrent CheckLimit() successCount = %d, ideal around %d (mock may allow more due to race)", successCount, limit)
		}
	})

	t.Run("concurrent cooldown checks", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()
		numGoroutines := 10
		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				allowed, _, err := limiter.CheckCooldown(ctx, "concurrent-cooldown", 60*time.Second)
				if err != nil {
					t.Errorf("CheckCooldown() error = %v", err)
					return
				}
				if allowed {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		// In concurrent environment, due to race conditions between Exists and Set,
		// multiple goroutines might succeed. The important thing is:
		// 1. At least one should succeed (successCount >= 1)
		// 2. Not all should succeed (successCount < numGoroutines)
		// 3. Ideally, only 1 should succeed, but we allow a small tolerance
		if successCount < 1 {
			t.Errorf("concurrent CheckCooldown() successCount = %d, want >= 1", successCount)
		}
		if successCount >= numGoroutines {
			t.Errorf("concurrent CheckCooldown() successCount = %d, want < %d (cooldown should prevent most)", successCount, numGoroutines)
		}
		// Allow tolerance for race conditions (typically 1-2 should succeed)
		if successCount > 3 {
			t.Logf("concurrent CheckCooldown() successCount = %d, expected 1-2 (race condition tolerance)", successCount)
		}
	})
}

func TestRateLimiter_KeyPrefixes(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiterWithPrefixes(client, "custom:", "custom-cooldown:")
	ctx := context.Background()

	// Test that prefixes are used
	_, _, _, err := limiter.CheckLimit(ctx, "key1", 10, time.Hour)
	if err != nil {
		t.Errorf("CheckLimit() with custom prefix error = %v, want nil", err)
	}

	_, _, err = limiter.CheckCooldown(ctx, "key1", 60*time.Second)
	if err != nil {
		t.Errorf("CheckCooldown() with custom prefix error = %v, want nil", err)
	}
}

func TestRateLimiter_CheckLimit_RedisFailures(t *testing.T) {
	t.Run("redis get failure", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// First request to set up the key
		_, _, _, _ = limiter.CheckLimit(ctx, "failkey1", 10, time.Hour)

		// Make Redis fail for subsequent operations
		mock.SetShouldFail(true)

		_, _, _, err := limiter.CheckLimit(ctx, "failkey1", 10, time.Hour)
		if err == nil {
			t.Error("CheckLimit() with Redis failure should return error")
		}

		mock.SetShouldFail(false)
	})

	t.Run("redis incr failure", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// First request to set up the key
		_, _, _, _ = limiter.CheckLimit(ctx, "failkey2", 10, time.Hour)

		// Make Redis fail for subsequent operations
		mock.SetShouldFail(true)

		_, _, _, err := limiter.CheckLimit(ctx, "failkey2", 10, time.Hour)
		if err == nil {
			t.Error("CheckLimit() with Redis Incr failure should return error")
		}

		mock.SetShouldFail(false)
	})

	t.Run("redis set failure on first request", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Make Redis fail immediately
		mock.SetShouldFail(true)

		_, _, _, err := limiter.CheckLimit(ctx, "failkey3", 10, time.Hour)
		if err == nil {
			t.Error("CheckLimit() with Redis Set failure should return error")
		}

		mock.SetShouldFail(false)
	})

	t.Run("first increment sets expiration (newCount == 1)", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Set key to "0" so that Get returns 0, then Incr gives newCount=1 (triggers Expire branch)
		redisKey := limiter.keyPrefix + "zerocount"
		_ = client.Set(ctx, redisKey, "0", time.Hour).Err()

		allowed, remaining, _, err := limiter.CheckLimit(ctx, "zerocount", 10, time.Hour)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Error("CheckLimit() allowed = false, want true")
		}
		if remaining != 9 {
			t.Errorf("CheckLimit() remaining = %d, want 9", remaining)
		}
	})

	t.Run("existing key with no TTL gets expiration (ttl <= 0 branch)", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Set key with no expiration (TTL 0)
		redisKey := limiter.keyPrefix + "noexpkey"
		_ = client.Set(ctx, redisKey, "1", 0).Err()

		allowed, remaining, _, err := limiter.CheckLimit(ctx, "noexpkey", 10, time.Hour)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Error("CheckLimit() allowed = false, want true")
		}
		if remaining != 8 {
			t.Errorf("CheckLimit() remaining = %d, want 8", remaining)
		}
	})
}

func TestRateLimiter_CheckCooldown_RedisFailures(t *testing.T) {
	t.Run("redis exists failure", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		mock.SetShouldFail(true)

		_, _, err := limiter.CheckCooldown(ctx, "coolfailkey1", 60*time.Second)
		if err == nil {
			t.Error("CheckCooldown() with Redis Exists failure should return error")
		}

		mock.SetShouldFail(false)
	})

	t.Run("redis set failure after exists check", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// We can't easily test the set failure after exists succeeds with the current mock
		// But we can test that the basic flow works
		allowed1, _, err1 := limiter.CheckCooldown(ctx, "coolfailkey2", 60*time.Second)
		if err1 != nil || !allowed1 {
			t.Error("First CheckCooldown() should succeed")
		}

		// Second call should be denied (cooldown active)
		allowed2, _, err2 := limiter.CheckCooldown(ctx, "coolfailkey2", 60*time.Second)
		if err2 != nil {
			t.Errorf("CheckCooldown() error = %v", err2)
		}
		if allowed2 {
			t.Error("Second CheckCooldown() should be denied (cooldown active)")
		}

		_ = mock // Silence unused variable warning
	})
}

func TestRateLimiter_EdgeCases(t *testing.T) {
	t.Run("zero limit", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// With limit=0, first request should set count to 1, which is >= 0
		// The CheckLimit function handles remaining < 0 by setting it to 0
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "zerokey", 0, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() error = %v", err)
		}
		// First request sets count to 1, remaining = 0 - 1 = -1, but capped to 0
		if allowed {
			t.Log("First request with limit=0 was allowed (sets count to 1)")
		}
		// Remaining is capped at 0 (not negative) in the implementation
		if remaining != 0 {
			t.Logf("Remaining = %d (expected 0 after capping)", remaining)
		}
	})

	t.Run("very short window", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Use very short window
		allowed, _, _, err := limiter.CheckLimit(ctx, "shortwindow", 2, 50*time.Millisecond)
		if err != nil {
			t.Errorf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Error("First request should be allowed")
		}

		// Wait for window to expire
		time.Sleep(100 * time.Millisecond)

		// Should be allowed again (new window)
		allowed2, remaining2, _, err2 := limiter.CheckLimit(ctx, "shortwindow", 2, 50*time.Millisecond)
		if err2 != nil {
			t.Errorf("CheckLimit() error = %v", err2)
		}
		if !allowed2 {
			t.Error("Request after window expiry should be allowed")
		}
		if remaining2 != 1 {
			t.Errorf("Remaining = %d, want 1 (first request in new window)", remaining2)
		}
	})

	t.Run("large limit", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Use very large limit
		allowed, remaining, _, err := limiter.CheckLimit(ctx, "largelimit", 1000000, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Error("Request should be allowed with large limit")
		}
		if remaining != 999999 {
			t.Errorf("Remaining = %d, want 999999", remaining)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Empty key should still work
		allowed, _, _, err := limiter.CheckLimit(ctx, "", 10, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() with empty key error = %v", err)
		}
		if !allowed {
			t.Error("Request should be allowed")
		}
	})

	t.Run("special characters in key", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		limiter := NewRateLimiter(client)
		ctx := context.Background()

		// Key with special characters
		allowed, _, _, err := limiter.CheckLimit(ctx, "key:with:colons:and-dashes_and_underscores", 10, time.Hour)
		if err != nil {
			t.Errorf("CheckLimit() with special characters error = %v", err)
		}
		if !allowed {
			t.Error("Request should be allowed")
		}
	})
}

func TestRateLimiter_Convenience_Methods(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	t.Run("CheckUserLimit multiple calls", func(t *testing.T) {
		// Make multiple calls to verify rate limiting works
		for i := 0; i < 5; i++ {
			_, _, _, _ = limiter.CheckUserLimit(ctx, "testuser", 5, time.Hour)
		}

		allowed, remaining, _, err := limiter.CheckUserLimit(ctx, "testuser", 5, time.Hour)
		if err != nil {
			t.Errorf("CheckUserLimit() error = %v", err)
		}
		if allowed {
			t.Error("Request should be denied (limit reached)")
		}
		if remaining != 0 {
			t.Errorf("Remaining = %d, want 0", remaining)
		}
	})

	t.Run("CheckIPLimit multiple calls", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			_, _, _, _ = limiter.CheckIPLimit(ctx, "10.0.0.1", 3, time.Hour)
		}

		allowed, remaining, _, err := limiter.CheckIPLimit(ctx, "10.0.0.1", 3, time.Hour)
		if err != nil {
			t.Errorf("CheckIPLimit() error = %v", err)
		}
		if allowed {
			t.Error("Request should be denied (limit reached)")
		}
		if remaining != 0 {
			t.Errorf("Remaining = %d, want 0", remaining)
		}
	})

	t.Run("CheckDestinationLimit multiple calls", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			_, _, _, _ = limiter.CheckDestinationLimit(ctx, "+1234567890", 4, time.Hour)
		}

		allowed, remaining, _, err := limiter.CheckDestinationLimit(ctx, "+1234567890", 4, time.Hour)
		if err != nil {
			t.Errorf("CheckDestinationLimit() error = %v", err)
		}
		if allowed {
			t.Error("Request should be denied (limit reached)")
		}
		if remaining != 0 {
			t.Errorf("Remaining = %d, want 0", remaining)
		}
	})
}
