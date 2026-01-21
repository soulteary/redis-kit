package client

import (
	"time"
)

// Config represents Redis client configuration
type Config struct {
	// Addr is the Redis server address (e.g., "localhost:6379")
	Addr string

	// Password is the Redis password (empty if no password)
	Password string

	// DB is the Redis database number (default: 0)
	DB int

	// PoolSize is the maximum number of socket connections (default: 10)
	PoolSize int

	// MinIdleConns is the minimum number of idle connections (default: 5)
	MinIdleConns int

	// DialTimeout is the timeout for establishing connections (default: 5s)
	DialTimeout time.Duration

	// ReadTimeout is the timeout for socket reads (default: 3s)
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for socket writes (default: 3s)
	WriteTimeout time.Duration

	// MaxRetries is the maximum number of retries for failed commands (default: 3)
	MaxRetries int

	// PoolTimeout is the timeout for getting a connection from the pool (default: 4s)
	PoolTimeout time.Duration
}

// DefaultConfig returns a Config with default values
func DefaultConfig() Config {
	return Config{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		MaxRetries:   3,
		PoolTimeout:  4 * time.Second,
	}
}

// WithAddr sets the Redis server address
func (c Config) WithAddr(addr string) Config {
	c.Addr = addr
	return c
}

// WithPassword sets the Redis password
func (c Config) WithPassword(password string) Config {
	c.Password = password
	return c
}

// WithDB sets the Redis database number
func (c Config) WithDB(db int) Config {
	c.DB = db
	return c
}

// WithPoolSize sets the connection pool size
func (c Config) WithPoolSize(size int) Config {
	c.PoolSize = size
	return c
}

// WithMinIdleConns sets the minimum number of idle connections
func (c Config) WithMinIdleConns(minIdle int) Config {
	c.MinIdleConns = minIdle
	return c
}

// WithDialTimeout sets the dial timeout
func (c Config) WithDialTimeout(timeout time.Duration) Config {
	c.DialTimeout = timeout
	return c
}

// WithReadTimeout sets the read timeout
func (c Config) WithReadTimeout(timeout time.Duration) Config {
	c.ReadTimeout = timeout
	return c
}

// WithWriteTimeout sets the write timeout
func (c Config) WithWriteTimeout(timeout time.Duration) Config {
	c.WriteTimeout = timeout
	return c
}

// WithMaxRetries sets the maximum number of retries
func (c Config) WithMaxRetries(retries int) Config {
	c.MaxRetries = retries
	return c
}

// WithPoolTimeout sets the pool timeout
func (c Config) WithPoolTimeout(timeout time.Duration) Config {
	c.PoolTimeout = timeout
	return c
}
