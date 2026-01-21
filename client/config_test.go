package client

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Addr != "localhost:6379" {
		t.Errorf("DefaultConfig().Addr = %q, want %q", cfg.Addr, "localhost:6379")
	}
	if cfg.Password != "" {
		t.Errorf("DefaultConfig().Password = %q, want empty", cfg.Password)
	}
	if cfg.DB != 0 {
		t.Errorf("DefaultConfig().DB = %d, want 0", cfg.DB)
	}
	if cfg.PoolSize != 10 {
		t.Errorf("DefaultConfig().PoolSize = %d, want 10", cfg.PoolSize)
	}
	if cfg.MinIdleConns != 5 {
		t.Errorf("DefaultConfig().MinIdleConns = %d, want 5", cfg.MinIdleConns)
	}
	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("DefaultConfig().DialTimeout = %v, want %v", cfg.DialTimeout, 5*time.Second)
	}
	if cfg.ReadTimeout != 3*time.Second {
		t.Errorf("DefaultConfig().ReadTimeout = %v, want %v", cfg.ReadTimeout, 3*time.Second)
	}
	if cfg.WriteTimeout != 3*time.Second {
		t.Errorf("DefaultConfig().WriteTimeout = %v, want %v", cfg.WriteTimeout, 3*time.Second)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("DefaultConfig().MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.PoolTimeout != 4*time.Second {
		t.Errorf("DefaultConfig().PoolTimeout = %v, want %v", cfg.PoolTimeout, 4*time.Second)
	}
}

func TestWithAddr(t *testing.T) {
	cfg := DefaultConfig().WithAddr("127.0.0.1:6380")
	if cfg.Addr != "127.0.0.1:6380" {
		t.Errorf("WithAddr() = %q, want %q", cfg.Addr, "127.0.0.1:6380")
	}

	// Verify immutability
	cfg2 := cfg.WithAddr("192.168.1.1:6379")
	if cfg.Addr != "127.0.0.1:6380" {
		t.Error("WithAddr() should not modify original config")
	}
	if cfg2.Addr != "192.168.1.1:6379" {
		t.Errorf("WithAddr() = %q, want %q", cfg2.Addr, "192.168.1.1:6379")
	}
}

func TestWithPassword(t *testing.T) {
	cfg := DefaultConfig().WithPassword("mypassword")
	if cfg.Password != "mypassword" {
		t.Errorf("WithPassword() = %q, want %q", cfg.Password, "mypassword")
	}

	// Verify immutability
	cfg2 := cfg.WithPassword("newpassword")
	if cfg.Password != "mypassword" {
		t.Error("WithPassword() should not modify original config")
	}
	if cfg2.Password != "newpassword" {
		t.Errorf("WithPassword() = %q, want %q", cfg2.Password, "newpassword")
	}
}

func TestWithDB(t *testing.T) {
	cfg := DefaultConfig().WithDB(1)
	if cfg.DB != 1 {
		t.Errorf("WithDB() = %d, want 1", cfg.DB)
	}

	// Verify immutability
	cfg2 := cfg.WithDB(2)
	if cfg.DB != 1 {
		t.Error("WithDB() should not modify original config")
	}
	if cfg2.DB != 2 {
		t.Errorf("WithDB() = %d, want 2", cfg2.DB)
	}
}

func TestWithPoolSize(t *testing.T) {
	cfg := DefaultConfig().WithPoolSize(20)
	if cfg.PoolSize != 20 {
		t.Errorf("WithPoolSize() = %d, want 20", cfg.PoolSize)
	}

	// Verify immutability
	cfg2 := cfg.WithPoolSize(30)
	if cfg.PoolSize != 20 {
		t.Error("WithPoolSize() should not modify original config")
	}
	if cfg2.PoolSize != 30 {
		t.Errorf("WithPoolSize() = %d, want 30", cfg2.PoolSize)
	}
}

func TestWithMinIdleConns(t *testing.T) {
	cfg := DefaultConfig().WithMinIdleConns(10)
	if cfg.MinIdleConns != 10 {
		t.Errorf("WithMinIdleConns() = %d, want 10", cfg.MinIdleConns)
	}

	// Verify immutability
	cfg2 := cfg.WithMinIdleConns(15)
	if cfg.MinIdleConns != 10 {
		t.Error("WithMinIdleConns() should not modify original config")
	}
	if cfg2.MinIdleConns != 15 {
		t.Errorf("WithMinIdleConns() = %d, want 15", cfg2.MinIdleConns)
	}
}

func TestWithDialTimeout(t *testing.T) {
	timeout := 10 * time.Second
	cfg := DefaultConfig().WithDialTimeout(timeout)
	if cfg.DialTimeout != timeout {
		t.Errorf("WithDialTimeout() = %v, want %v", cfg.DialTimeout, timeout)
	}

	// Verify immutability
	newTimeout := 15 * time.Second
	cfg2 := cfg.WithDialTimeout(newTimeout)
	if cfg.DialTimeout != timeout {
		t.Error("WithDialTimeout() should not modify original config")
	}
	if cfg2.DialTimeout != newTimeout {
		t.Errorf("WithDialTimeout() = %v, want %v", cfg2.DialTimeout, newTimeout)
	}
}

func TestWithReadTimeout(t *testing.T) {
	timeout := 5 * time.Second
	cfg := DefaultConfig().WithReadTimeout(timeout)
	if cfg.ReadTimeout != timeout {
		t.Errorf("WithReadTimeout() = %v, want %v", cfg.ReadTimeout, timeout)
	}

	// Verify immutability
	newTimeout := 6 * time.Second
	cfg2 := cfg.WithReadTimeout(newTimeout)
	if cfg.ReadTimeout != timeout {
		t.Error("WithReadTimeout() should not modify original config")
	}
	if cfg2.ReadTimeout != newTimeout {
		t.Errorf("WithReadTimeout() = %v, want %v", cfg2.ReadTimeout, newTimeout)
	}
}

func TestWithWriteTimeout(t *testing.T) {
	timeout := 5 * time.Second
	cfg := DefaultConfig().WithWriteTimeout(timeout)
	if cfg.WriteTimeout != timeout {
		t.Errorf("WithWriteTimeout() = %v, want %v", cfg.WriteTimeout, timeout)
	}

	// Verify immutability
	newTimeout := 6 * time.Second
	cfg2 := cfg.WithWriteTimeout(newTimeout)
	if cfg.WriteTimeout != timeout {
		t.Error("WithWriteTimeout() should not modify original config")
	}
	if cfg2.WriteTimeout != newTimeout {
		t.Errorf("WithWriteTimeout() = %v, want %v", cfg2.WriteTimeout, newTimeout)
	}
}

func TestWithMaxRetries(t *testing.T) {
	cfg := DefaultConfig().WithMaxRetries(5)
	if cfg.MaxRetries != 5 {
		t.Errorf("WithMaxRetries() = %d, want 5", cfg.MaxRetries)
	}

	// Verify immutability
	cfg2 := cfg.WithMaxRetries(10)
	if cfg.MaxRetries != 5 {
		t.Error("WithMaxRetries() should not modify original config")
	}
	if cfg2.MaxRetries != 10 {
		t.Errorf("WithMaxRetries() = %d, want 10", cfg2.MaxRetries)
	}
}

func TestWithPoolTimeout(t *testing.T) {
	timeout := 8 * time.Second
	cfg := DefaultConfig().WithPoolTimeout(timeout)
	if cfg.PoolTimeout != timeout {
		t.Errorf("WithPoolTimeout() = %v, want %v", cfg.PoolTimeout, timeout)
	}

	// Verify immutability
	newTimeout := 10 * time.Second
	cfg2 := cfg.WithPoolTimeout(newTimeout)
	if cfg.PoolTimeout != timeout {
		t.Error("WithPoolTimeout() should not modify original config")
	}
	if cfg2.PoolTimeout != newTimeout {
		t.Errorf("WithPoolTimeout() = %v, want %v", cfg2.PoolTimeout, newTimeout)
	}
}

func TestConfigChaining(t *testing.T) {
	cfg := DefaultConfig().
		WithAddr("127.0.0.1:6379").
		WithPassword("password").
		WithDB(1).
		WithPoolSize(20).
		WithMinIdleConns(10).
		WithDialTimeout(10 * time.Second).
		WithReadTimeout(5 * time.Second).
		WithWriteTimeout(5 * time.Second).
		WithMaxRetries(5).
		WithPoolTimeout(8 * time.Second)

	if cfg.Addr != "127.0.0.1:6379" {
		t.Errorf("chained WithAddr() = %q, want %q", cfg.Addr, "127.0.0.1:6379")
	}
	if cfg.Password != "password" {
		t.Errorf("chained WithPassword() = %q, want %q", cfg.Password, "password")
	}
	if cfg.DB != 1 {
		t.Errorf("chained WithDB() = %d, want 1", cfg.DB)
	}
	if cfg.PoolSize != 20 {
		t.Errorf("chained WithPoolSize() = %d, want 20", cfg.PoolSize)
	}
	if cfg.MinIdleConns != 10 {
		t.Errorf("chained WithMinIdleConns() = %d, want 10", cfg.MinIdleConns)
	}
	if cfg.DialTimeout != 10*time.Second {
		t.Errorf("chained WithDialTimeout() = %v, want %v", cfg.DialTimeout, 10*time.Second)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("chained WithReadTimeout() = %v, want %v", cfg.ReadTimeout, 5*time.Second)
	}
	if cfg.WriteTimeout != 5*time.Second {
		t.Errorf("chained WithWriteTimeout() = %v, want %v", cfg.WriteTimeout, 5*time.Second)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("chained WithMaxRetries() = %d, want 5", cfg.MaxRetries)
	}
	if cfg.PoolTimeout != 8*time.Second {
		t.Errorf("chained WithPoolTimeout() = %v, want %v", cfg.PoolTimeout, 8*time.Second)
	}
}
