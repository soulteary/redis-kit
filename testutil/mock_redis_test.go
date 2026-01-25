package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestNewMockRedis(t *testing.T) {
	mock := NewMockRedis()
	if mock == nil {
		t.Fatal("NewMockRedis() returned nil")
	}
	if mock.data == nil {
		t.Error("NewMockRedis() data map is nil")
	}
}

func TestMockRedis_SetShouldFail(t *testing.T) {
	mock := NewMockRedis()

	// Default should be false
	if mock.shouldFail {
		t.Error("shouldFail should be false by default")
	}

	// Set to true
	mock.SetShouldFail(true)
	if !mock.shouldFail {
		t.Error("shouldFail should be true after SetShouldFail(true)")
	}

	// Set back to false
	mock.SetShouldFail(false)
	if mock.shouldFail {
		t.Error("shouldFail should be false after SetShouldFail(false)")
	}
}

func TestNewMockRedisClient(t *testing.T) {
	client, mock := NewMockRedisClient()
	if client == nil {
		t.Fatal("NewMockRedisClient() client is nil")
	}
	if mock == nil {
		t.Fatal("NewMockRedisClient() mock is nil")
	}
	defer func() { _ = client.Close() }()

	// Test PING
	ctx := context.Background()
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}
	if pong != "PONG" {
		t.Errorf("Ping() = %q, want %q", pong, "PONG")
	}
}

func TestMockRedis_SET_GET(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("basic set and get", func(t *testing.T) {
		err := client.Set(ctx, "key1", "value1", 0).Err()
		if err != nil {
			t.Errorf("Set() error = %v", err)
		}

		val, err := client.Get(ctx, "key1").Result()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if val != "value1" {
			t.Errorf("Get() = %q, want %q", val, "value1")
		}
	})

	t.Run("set with TTL", func(t *testing.T) {
		err := client.Set(ctx, "key2", "value2", 1*time.Hour).Err()
		if err != nil {
			t.Errorf("Set() with TTL error = %v", err)
		}

		val, err := client.Get(ctx, "key2").Result()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if val != "value2" {
			t.Errorf("Get() = %q, want %q", val, "value2")
		}
	})

	t.Run("get non-existent key", func(t *testing.T) {
		_, err := client.Get(ctx, "nonexistent").Result()
		if err == nil {
			t.Error("Get() non-existent key should return error")
		}
	})

	t.Run("set with expiration", func(t *testing.T) {
		err := client.Set(ctx, "expiring", "value", 50*time.Millisecond).Err()
		if err != nil {
			t.Errorf("Set() error = %v", err)
		}

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		_, err = client.Get(ctx, "expiring").Result()
		if err == nil {
			t.Error("Get() on expired key should return error")
		}
	})
}

func TestMockRedis_SetNX(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("setnx on new key", func(t *testing.T) {
		success, err := client.SetNX(ctx, "nxkey1", "value1", 1*time.Hour).Result()
		if err != nil {
			t.Errorf("SetNX() error = %v", err)
		}
		if !success {
			t.Error("SetNX() on new key = false, want true")
		}

		val, _ := client.Get(ctx, "nxkey1").Result()
		if val != "value1" {
			t.Errorf("Get() after SetNX = %q, want %q", val, "value1")
		}
	})

	t.Run("setnx on existing key", func(t *testing.T) {
		// First set
		_, _ = client.SetNX(ctx, "nxkey2", "value1", 1*time.Hour).Result()

		// Second set should fail
		success, err := client.SetNX(ctx, "nxkey2", "value2", 1*time.Hour).Result()
		if err != nil {
			t.Errorf("SetNX() error = %v", err)
		}
		if success {
			t.Error("SetNX() on existing key = true, want false")
		}

		// Value should be unchanged
		val, _ := client.Get(ctx, "nxkey2").Result()
		if val != "value1" {
			t.Errorf("Get() after failed SetNX = %q, want %q (original value)", val, "value1")
		}
	})

	t.Run("setnx on expired key", func(t *testing.T) {
		// Set with short TTL
		_, _ = client.SetNX(ctx, "nxkey3", "value1", 50*time.Millisecond).Result()

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		// SetNX should succeed on expired key
		success, err := client.SetNX(ctx, "nxkey3", "value2", 1*time.Hour).Result()
		if err != nil {
			t.Errorf("SetNX() on expired key error = %v", err)
		}
		if !success {
			t.Error("SetNX() on expired key = false, want true")
		}
	})
}

func TestMockRedis_DEL(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("delete existing key", func(t *testing.T) {
		_ = client.Set(ctx, "delkey1", "value1", 0).Err()

		deleted, err := client.Del(ctx, "delkey1").Result()
		if err != nil {
			t.Errorf("Del() error = %v", err)
		}
		if deleted != 1 {
			t.Errorf("Del() = %d, want 1", deleted)
		}

		// Verify key is deleted
		_, err = client.Get(ctx, "delkey1").Result()
		if err == nil {
			t.Error("Get() after Del() should return error")
		}
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		deleted, err := client.Del(ctx, "nonexistent").Result()
		if err != nil {
			t.Errorf("Del() error = %v", err)
		}
		if deleted != 0 {
			t.Errorf("Del() non-existent key = %d, want 0", deleted)
		}
	})

	t.Run("delete multiple keys", func(t *testing.T) {
		_ = client.Set(ctx, "multi1", "v1", 0).Err()
		_ = client.Set(ctx, "multi2", "v2", 0).Err()
		_ = client.Set(ctx, "multi3", "v3", 0).Err()

		deleted, err := client.Del(ctx, "multi1", "multi2", "multi3", "nonexistent").Result()
		if err != nil {
			t.Errorf("Del() error = %v", err)
		}
		if deleted != 3 {
			t.Errorf("Del() multiple keys = %d, want 3", deleted)
		}
	})
}

func TestMockRedis_EXISTS(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("exists for existing key", func(t *testing.T) {
		_ = client.Set(ctx, "existkey1", "value1", 0).Err()

		count, err := client.Exists(ctx, "existkey1").Result()
		if err != nil {
			t.Errorf("Exists() error = %v", err)
		}
		if count != 1 {
			t.Errorf("Exists() = %d, want 1", count)
		}
	})

	t.Run("exists for non-existent key", func(t *testing.T) {
		count, err := client.Exists(ctx, "nonexistent").Result()
		if err != nil {
			t.Errorf("Exists() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Exists() non-existent = %d, want 0", count)
		}
	})

	t.Run("exists for multiple keys", func(t *testing.T) {
		_ = client.Set(ctx, "mexist1", "v1", 0).Err()
		_ = client.Set(ctx, "mexist2", "v2", 0).Err()

		count, err := client.Exists(ctx, "mexist1", "mexist2", "nonexistent").Result()
		if err != nil {
			t.Errorf("Exists() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Exists() multiple keys = %d, want 2", count)
		}
	})

	t.Run("exists for expired key", func(t *testing.T) {
		_ = client.Set(ctx, "expexist", "value", 50*time.Millisecond).Err()

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		count, err := client.Exists(ctx, "expexist").Result()
		if err != nil {
			t.Errorf("Exists() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Exists() expired key = %d, want 0", count)
		}
	})
}

func TestMockRedis_INCR(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("incr new key", func(t *testing.T) {
		val, err := client.Incr(ctx, "incrkey1").Result()
		if err != nil {
			t.Errorf("Incr() error = %v", err)
		}
		if val != 1 {
			t.Errorf("Incr() new key = %d, want 1", val)
		}
	})

	t.Run("incr existing numeric key", func(t *testing.T) {
		_ = client.Set(ctx, "incrkey2", "10", 0).Err()

		val, err := client.Incr(ctx, "incrkey2").Result()
		if err != nil {
			t.Errorf("Incr() error = %v", err)
		}
		if val != 11 {
			t.Errorf("Incr() existing key = %d, want 11", val)
		}
	})

	t.Run("incr non-numeric key", func(t *testing.T) {
		_ = client.Set(ctx, "incrkey3", "notanumber", 0).Err()

		_, err := client.Incr(ctx, "incrkey3").Result()
		if err == nil {
			t.Error("Incr() non-numeric key should return error")
		}
	})

	t.Run("multiple incr", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			_, _ = client.Incr(ctx, "incrkey4").Result()
		}

		val, err := client.Incr(ctx, "incrkey4").Result()
		if err != nil {
			t.Errorf("Incr() error = %v", err)
		}
		if val != 6 {
			t.Errorf("Incr() after 5 increments = %d, want 6", val)
		}
	})
}

func TestMockRedis_TTL(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("ttl for key with expiration", func(t *testing.T) {
		_ = client.Set(ctx, "ttlkey1", "value", 1*time.Hour).Err()

		ttl, err := client.TTL(ctx, "ttlkey1").Result()
		if err != nil {
			t.Errorf("TTL() error = %v", err)
		}
		if ttl <= 0 {
			t.Errorf("TTL() = %v, want positive", ttl)
		}
		if ttl > 1*time.Hour+time.Second {
			t.Errorf("TTL() = %v, should be <= 1 hour", ttl)
		}
	})

	t.Run("ttl for key without expiration", func(t *testing.T) {
		_ = client.Set(ctx, "ttlkey2", "value", 0).Err()

		ttl, err := client.TTL(ctx, "ttlkey2").Result()
		if err != nil {
			t.Errorf("TTL() error = %v", err)
		}
		// Redis returns -1 for keys with no expiration
		// go-redis interprets this as -1 nanosecond
		if ttl >= 0 {
			t.Errorf("TTL() no expiration = %v, want negative (no expiration)", ttl)
		}
	})

	t.Run("ttl for non-existent key", func(t *testing.T) {
		ttl, err := client.TTL(ctx, "nonexistent").Result()
		if err != nil {
			t.Errorf("TTL() error = %v", err)
		}
		// Redis returns -2 for non-existent keys
		// go-redis interprets this as -2 nanoseconds
		if ttl >= 0 {
			t.Errorf("TTL() non-existent = %v, want negative (non-existent)", ttl)
		}
	})

	t.Run("ttl for expired key", func(t *testing.T) {
		_ = client.Set(ctx, "ttlkey3", "value", 50*time.Millisecond).Err()

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		ttl, err := client.TTL(ctx, "ttlkey3").Result()
		if err != nil {
			t.Errorf("TTL() error = %v", err)
		}
		// Redis returns -2 for expired keys (treated as non-existent)
		if ttl >= 0 {
			t.Errorf("TTL() expired key = %v, want negative (expired/non-existent)", ttl)
		}
	})
}

func TestMockRedis_EXPIRE(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("expire existing key", func(t *testing.T) {
		_ = client.Set(ctx, "expkey1", "value", 0).Err()

		success, err := client.Expire(ctx, "expkey1", 1*time.Hour).Result()
		if err != nil {
			t.Errorf("Expire() error = %v", err)
		}
		if !success {
			t.Error("Expire() existing key = false, want true")
		}

		// Verify TTL is set
		ttl, _ := client.TTL(ctx, "expkey1").Result()
		if ttl <= 0 {
			t.Errorf("TTL() after Expire() = %v, want positive", ttl)
		}
	})

	t.Run("expire non-existent key", func(t *testing.T) {
		success, err := client.Expire(ctx, "nonexistent", 1*time.Hour).Result()
		if err != nil {
			t.Errorf("Expire() error = %v", err)
		}
		if success {
			t.Error("Expire() non-existent key = true, want false")
		}
	})
}

func TestMockRedis_EVAL(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Test the unlock Lua script
	t.Run("eval unlock script - value matches", func(t *testing.T) {
		_ = client.Set(ctx, "lockkey1", "lockvalue1", 0).Err()

		script := `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		result, err := client.Eval(ctx, script, []string{"lockkey1"}, "lockvalue1").Result()
		if err != nil {
			t.Errorf("Eval() error = %v", err)
		}
		if result.(int64) != 1 {
			t.Errorf("Eval() unlock success = %v, want 1", result)
		}

		// Key should be deleted
		_, err = client.Get(ctx, "lockkey1").Result()
		if err == nil {
			t.Error("Key should be deleted after unlock")
		}
	})

	t.Run("eval unlock script - value mismatch", func(t *testing.T) {
		_ = client.Set(ctx, "lockkey2", "lockvalue1", 0).Err()

		script := `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		result, err := client.Eval(ctx, script, []string{"lockkey2"}, "wrongvalue").Result()
		if err != nil {
			t.Errorf("Eval() error = %v", err)
		}
		if result.(int64) != 0 {
			t.Errorf("Eval() unlock mismatch = %v, want 0", result)
		}

		// Key should still exist
		val, err := client.Get(ctx, "lockkey2").Result()
		if err != nil || val != "lockvalue1" {
			t.Error("Key should still exist after failed unlock")
		}
	})

	t.Run("eval unlock script - key not found", func(t *testing.T) {
		script := `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		result, err := client.Eval(ctx, script, []string{"nonexistent"}, "lockvalue").Result()
		if err != nil {
			t.Errorf("Eval() error = %v", err)
		}
		if result.(int64) != 0 {
			t.Errorf("Eval() unlock nonexistent = %v, want 0", result)
		}
	})
}

func TestMockRedis_FLUSHDB(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Set some keys
	_ = client.Set(ctx, "flush1", "v1", 0).Err()
	_ = client.Set(ctx, "flush2", "v2", 0).Err()
	_ = client.Set(ctx, "flush3", "v3", 0).Err()

	// Flush
	err := client.FlushDB(ctx).Err()
	if err != nil {
		t.Errorf("FlushDB() error = %v", err)
	}

	// All keys should be gone
	count, _ := client.Exists(ctx, "flush1", "flush2", "flush3").Result()
	if count != 0 {
		t.Errorf("Exists() after FlushDB = %d, want 0", count)
	}
}

func TestMockRedis_ShouldFail(t *testing.T) {
	client, mock := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("operations fail when shouldFail is true", func(t *testing.T) {
		mock.SetShouldFail(true)
		defer mock.SetShouldFail(false)

		err := client.Set(ctx, "key", "value", 0).Err()
		if err == nil {
			t.Error("Set() should fail when shouldFail is true")
		}

		_, err = client.Get(ctx, "key").Result()
		if err == nil {
			t.Error("Get() should fail when shouldFail is true")
		}

		err = client.Ping(ctx).Err()
		if err == nil {
			t.Error("Ping() should fail when shouldFail is true")
		}
	})

	t.Run("operations succeed when shouldFail is false", func(t *testing.T) {
		mock.SetShouldFail(false)

		err := client.Set(ctx, "key2", "value2", 0).Err()
		if err != nil {
			t.Errorf("Set() error = %v, want nil", err)
		}

		val, err := client.Get(ctx, "key2").Result()
		if err != nil || val != "value2" {
			t.Errorf("Get() = %q, %v, want %q, nil", val, err, "value2")
		}
	})
}

func TestMockRedis_Concurrent(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Test concurrent operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			key := "concurrent:" + string(rune('a'+n))
			_ = client.Set(ctx, key, "value", 0).Err()
			_, _ = client.Get(ctx, key).Result()
			_, _ = client.Incr(ctx, key+"counter").Result()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMockRedis_SET_Options(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("set with NX option - key doesn't exist", func(t *testing.T) {
		// Use SetArgs to set with NX option
		args := &redis.SetArgs{
			Mode: "NX",
			TTL:  time.Hour,
		}
		result := client.SetArgs(ctx, "nxtest1", "value1", *args)
		err := result.Err()
		if err != nil {
			t.Errorf("SetArgs NX error = %v", err)
		}

		// Verify value was set
		val, _ := client.Get(ctx, "nxtest1").Result()
		if val != "value1" {
			t.Errorf("Get after SetArgs NX = %q, want %q", val, "value1")
		}
	})

	t.Run("set with NX option - key exists", func(t *testing.T) {
		// Set initial value
		_ = client.Set(ctx, "nxtest2", "original", 0).Err()

		// Try to set with NX (should fail silently, key not changed)
		args := &redis.SetArgs{
			Mode: "NX",
			TTL:  time.Hour,
		}
		_ = client.SetArgs(ctx, "nxtest2", "new", *args)

		// Value should be unchanged
		val, _ := client.Get(ctx, "nxtest2").Result()
		if val != "original" {
			t.Errorf("Get after failed SetArgs NX = %q, want %q", val, "original")
		}
	})

	t.Run("set with PX option", func(t *testing.T) {
		err := client.Set(ctx, "pxtest", "value", 500*time.Millisecond).Err()
		if err != nil {
			t.Errorf("Set with PX error = %v", err)
		}

		// Verify value exists
		val, err := client.Get(ctx, "pxtest").Result()
		if err != nil || val != "value" {
			t.Errorf("Get after Set with PX = %q, %v", val, err)
		}

		// Wait for expiration
		time.Sleep(600 * time.Millisecond)

		// Should be expired
		_, err = client.Get(ctx, "pxtest").Result()
		if err == nil {
			t.Error("Get on expired PX key should return error")
		}
	})
}

func TestMockRedis_EVAL_EdgeCases(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("eval unsupported script", func(t *testing.T) {
		// Try an unsupported script
		_, err := client.Eval(ctx, "return redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])", []string{"key"}, "field", "value").Result()
		if err == nil {
			t.Error("Eval with unsupported script should return error")
		}
	})
}

func TestMockRedis_UnknownCommand(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Try to use HSET which is not supported
	err := client.HSet(ctx, "hashkey", "field", "value").Err()
	if err == nil {
		t.Error("Unsupported command should return error")
	}
}

func TestMockRedis_Expire_EdgeCases(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("expire updates existing expiration", func(t *testing.T) {
		// Set with short TTL
		_ = client.Set(ctx, "expirekey", "value", 10*time.Second).Err()

		// Update to longer TTL
		success, err := client.Expire(ctx, "expirekey", 1*time.Hour).Result()
		if err != nil {
			t.Errorf("Expire() error = %v", err)
		}
		if !success {
			t.Error("Expire() = false, want true")
		}

		// TTL should be updated
		ttl, _ := client.TTL(ctx, "expirekey").Result()
		if ttl <= 10*time.Second {
			t.Errorf("TTL after Expire = %v, should be longer than 10s", ttl)
		}
	})
}

func TestMockRedis_DEL_EdgeCases(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	t.Run("delete single key", func(t *testing.T) {
		_ = client.Set(ctx, "singlekey", "value", 0).Err()

		deleted, err := client.Del(ctx, "singlekey").Result()
		if err != nil {
			t.Errorf("Del() error = %v", err)
		}
		if deleted != 1 {
			t.Errorf("Del() = %d, want 1", deleted)
		}
	})
}

func TestMockRedis_Incr_Preserves_TTL(t *testing.T) {
	client, _ := NewMockRedisClient()
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Set with TTL
	_ = client.Set(ctx, "incrttl", "5", 1*time.Hour).Err()

	// Incr
	_, _ = client.Incr(ctx, "incrttl").Result()

	// TTL should be preserved
	ttl, err := client.TTL(ctx, "incrttl").Result()
	if err != nil {
		t.Errorf("TTL() error = %v", err)
	}
	if ttl <= 0 {
		t.Errorf("TTL after Incr = %v, should be positive", ttl)
	}
}
