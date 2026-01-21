package cache

import (
	"context"
	"testing"
	"time"

	"github.com/soulteary/redis-kit/testutil"
)

func TestNewCache(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	c := NewCache(client, "test:")
	if c == nil {
		t.Fatal("NewCache() returned nil")
	}
	if c.client != client {
		t.Error("NewCache() client mismatch")
	}
	if c.keyPrefix != "test:" {
		t.Errorf("NewCache() keyPrefix = %q, want %q", c.keyPrefix, "test:")
	}
}

func TestRedisCache_buildKey(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		key := c.buildKey("mykey")
		expected := "test:mykey"
		if key != expected {
			t.Errorf("buildKey() = %q, want %q", key, expected)
		}
	})

	t.Run("without prefix", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "")
		key := c.buildKey("mykey")
		expected := "mykey"
		if key != expected {
			t.Errorf("buildKey() = %q, want %q", key, expected)
		}
	})
}

func TestRedisCache_Set(t *testing.T) {
	t.Run("successful set", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		err := c.Set(ctx, "key1", "value1", time.Minute)
		if err != nil {
			t.Errorf("Set() error = %v, want nil", err)
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		c := &RedisCache{
			client:    nil,
			keyPrefix: "test:",
		}
		ctx := context.Background()

		err := c.Set(ctx, "key1", "value1", time.Minute)
		if err == nil {
			t.Error("Set() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Set() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("complex type serialization", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		type User struct {
			ID   string
			Name string
			Age  int
		}

		user := User{ID: "123", Name: "Alice", Age: 30}
		err := c.Set(ctx, "user:123", user, time.Minute)
		if err != nil {
			t.Errorf("Set() with struct error = %v, want nil", err)
		}
	})

	t.Run("slice serialization", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		items := []string{"item1", "item2", "item3"}
		err := c.Set(ctx, "items", items, time.Minute)
		if err != nil {
			t.Errorf("Set() with slice error = %v, want nil", err)
		}
	})

	t.Run("map serialization", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		data := map[string]interface{}{
			"key1": "value1",
			"key2": 123,
			"key3": true,
		}
		err := c.Set(ctx, "map", data, time.Minute)
		if err != nil {
			t.Errorf("Set() with map error = %v, want nil", err)
		}
	})
}

func TestRedisCache_Get(t *testing.T) {
	t.Run("successful get", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set first
		err := c.Set(ctx, "key1", "value1", time.Minute)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		// Get
		var value string
		err = c.Get(ctx, "key1", &value)
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
		if value != "value1" {
			t.Errorf("Get() = %q, want %q", value, "value1")
		}
	})

	t.Run("key not found", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		var value string
		err := c.Get(ctx, "nonexistent", &value)
		if err == nil {
			t.Error("Get() with non-existent key should return error")
		}
		if err.Error() != "key not found: nonexistent" {
			t.Errorf("Get() error = %q, want %q", err.Error(), "key not found: nonexistent")
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		c := &RedisCache{
			client:    nil,
			keyPrefix: "test:",
		}
		ctx := context.Background()

		var value string
		err := c.Get(ctx, "key1", &value)
		if err == nil {
			t.Error("Get() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Get() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("complex type deserialization", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		type User struct {
			ID   string
			Name string
			Age  int
		}

		original := User{ID: "123", Name: "Alice", Age: 30}
		err := c.Set(ctx, "user:123", original, time.Minute)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		var retrieved User
		err = c.Get(ctx, "user:123", &retrieved)
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
		if retrieved.ID != original.ID || retrieved.Name != original.Name || retrieved.Age != original.Age {
			t.Errorf("Get() = %+v, want %+v", retrieved, original)
		}
	})

	t.Run("slice deserialization", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		original := []string{"item1", "item2", "item3"}
		err := c.Set(ctx, "items", original, time.Minute)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		var retrieved []string
		err = c.Get(ctx, "items", &retrieved)
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
		if len(retrieved) != len(original) {
			t.Errorf("Get() length = %d, want %d", len(retrieved), len(original))
		}
	})
}

func TestRedisCache_Del(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set first
		_ = c.Set(ctx, "key1", "value1", time.Minute)

		// Delete
		err := c.Del(ctx, "key1")
		if err != nil {
			t.Errorf("Del() error = %v, want nil", err)
		}

		// Verify deleted
		var value string
		err = c.Get(ctx, "key1", &value)
		if err == nil {
			t.Error("Get() after Del() should return error")
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		c := &RedisCache{
			client:    nil,
			keyPrefix: "test:",
		}
		ctx := context.Background()

		err := c.Del(ctx, "key1")
		if err == nil {
			t.Error("Del() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Del() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})
}

func TestRedisCache_Exists(t *testing.T) {
	t.Run("key exists", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set first
		_ = c.Set(ctx, "key1", "value1", time.Minute)

		// Check existence
		exists, err := c.Exists(ctx, "key1")
		if err != nil {
			t.Errorf("Exists() error = %v, want nil", err)
		}
		if !exists {
			t.Error("Exists() = false, want true")
		}
	})

	t.Run("key does not exist", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		exists, err := c.Exists(ctx, "nonexistent")
		if err != nil {
			t.Errorf("Exists() error = %v, want nil", err)
		}
		if exists {
			t.Error("Exists() = true, want false")
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		c := &RedisCache{
			client:    nil,
			keyPrefix: "test:",
		}
		ctx := context.Background()

		_, err := c.Exists(ctx, "key1")
		if err == nil {
			t.Error("Exists() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Exists() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})
}

func TestRedisCache_TTL(t *testing.T) {
	t.Run("get TTL", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set with TTL
		_ = c.Set(ctx, "key1", "value1", time.Minute)

		ttl, err := c.TTL(ctx, "key1")
		if err != nil {
			t.Errorf("TTL() error = %v, want nil", err)
		}
		if ttl <= 0 {
			t.Errorf("TTL() = %v, should be positive", ttl)
		}
		if ttl > time.Minute+time.Second {
			t.Errorf("TTL() = %v, should be approximately 1 minute", ttl)
		}
	})

	t.Run("key does not exist", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		ttl, err := c.TTL(ctx, "nonexistent")
		// TTL on non-existent key may return -2 or error, both are acceptable
		if err == nil && ttl >= 0 {
			t.Logf("TTL() on non-existent key returned %v (acceptable)", ttl)
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		c := &RedisCache{
			client:    nil,
			keyPrefix: "test:",
		}
		ctx := context.Background()

		_, err := c.TTL(ctx, "key1")
		if err == nil {
			t.Error("TTL() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("TTL() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})
}

func TestRedisCache_Expire(t *testing.T) {
	t.Run("set expiration", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set first
		_ = c.Set(ctx, "key1", "value1", time.Minute)

		// Set new expiration
		err := c.Expire(ctx, "key1", 2*time.Minute)
		if err != nil {
			t.Errorf("Expire() error = %v, want nil", err)
		}

		// Verify TTL is updated
		ttl, err := c.TTL(ctx, "key1")
		if err != nil {
			t.Errorf("TTL() after Expire() error = %v, want nil", err)
		}
		if ttl <= 0 {
			t.Errorf("TTL() after Expire() = %v, should be positive", ttl)
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		c := &RedisCache{
			client:    nil,
			keyPrefix: "test:",
		}
		ctx := context.Background()

		err := c.Expire(ctx, "key1", time.Minute)
		if err == nil {
			t.Error("Expire() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Expire() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})
}

func TestRedisCache_KeyPrefix(t *testing.T) {
	t.Run("prefix is applied", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "app:")
		ctx := context.Background()

		// Set with prefix
		err := c.Set(ctx, "key1", "value1", time.Minute)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		// Get should work with same key (prefix handled internally)
		var value string
		err = c.Get(ctx, "key1", &value)
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
		if value != "value1" {
			t.Errorf("Get() = %q, want %q", value, "value1")
		}
	})

	t.Run("different prefixes don't conflict", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c1 := NewCache(client, "app1:")
		c2 := NewCache(client, "app2:")
		ctx := context.Background()

		// Set same key in different caches
		_ = c1.Set(ctx, "key", "value1", time.Minute)
		_ = c2.Set(ctx, "key", "value2", time.Minute)

		// Get should return different values
		var v1, v2 string
		_ = c1.Get(ctx, "key", &v1)
		_ = c2.Get(ctx, "key", &v2)

		if v1 != "value1" {
			t.Errorf("c1.Get() = %q, want %q", v1, "value1")
		}
		if v2 != "value2" {
			t.Errorf("c2.Get() = %q, want %q", v2, "value2")
		}
	})

	t.Run("invalid JSON deserialization", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set data that can't be unmarshaled to target type
		// Set a map that doesn't match User structure
		invalidData := map[string]interface{}{"not": "a user", "missing": "id field"}
		_ = c.Set(ctx, "invalid", invalidData, time.Minute)

		// Try to get as struct - should fail unmarshaling
		type User struct {
			ID string `json:"id"`
		}
		var user User
		err := c.Get(ctx, "invalid", &user)
		// This might succeed but user.ID will be empty, or it might fail
		// The important thing is we test the unmarshal path
		if err != nil {
			// Error is acceptable - JSON doesn't match structure
			t.Logf("Get() with mismatched JSON returned error (acceptable): %v", err)
		}
	})

	t.Run("zero TTL", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set with zero TTL (no expiration)
		err := c.Set(ctx, "key1", "value1", 0)
		if err != nil {
			t.Errorf("Set() with zero TTL error = %v, want nil", err)
		}

		// Should still be able to get
		var value string
		err = c.Get(ctx, "key1", &value)
		if err != nil {
			t.Errorf("Get() after zero TTL Set() error = %v, want nil", err)
		}
		if value != "value1" {
			t.Errorf("Get() = %q, want %q", value, "value1")
		}
	})

	t.Run("empty value", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set empty string
		err := c.Set(ctx, "empty", "", time.Minute)
		if err != nil {
			t.Errorf("Set() with empty value error = %v, want nil", err)
		}

		// Get empty string
		var value string
		err = c.Get(ctx, "empty", &value)
		if err != nil {
			t.Errorf("Get() empty value error = %v, want nil", err)
		}
		if value != "" {
			t.Errorf("Get() = %q, want empty string", value)
		}
	})

	t.Run("nil value handling", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		c := NewCache(client, "test:")
		ctx := context.Background()

		// Set nil (should serialize to null)
		var nilValue *string = nil
		err := c.Set(ctx, "nil", nilValue, time.Minute)
		if err != nil {
			t.Errorf("Set() with nil value error = %v, want nil", err)
		}

		// Get nil
		var retrieved *string
		err = c.Get(ctx, "nil", &retrieved)
		if err != nil {
			t.Errorf("Get() nil value error = %v, want nil", err)
		}
		// Retrieved should be nil
		if retrieved != nil {
			t.Errorf("Get() = %v, want nil", retrieved)
		}
	})

	t.Run("buildKey edge cases", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		// Test with various prefix/key combinations
		testCases := []struct {
			prefix string
			key    string
			want   string
		}{
			{"", "", ""},
			{"", "key", "key"},
			{"prefix:", "", "prefix:"},
			{"prefix:", "key", "prefix:key"},
			{"a", "b", "ab"},
		}

		for _, tc := range testCases {
			c := NewCache(client, tc.prefix)
			got := c.buildKey(tc.key)
			if got != tc.want {
				t.Errorf("buildKey(prefix=%q, key=%q) = %q, want %q", tc.prefix, tc.key, got, tc.want)
			}
		}
	})
}
