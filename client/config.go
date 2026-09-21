package client

import (
	"context"
	"net"
	"time"
)

// Dialer is the type for custom Redis connection dialer (optional, for testing or custom network).
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

// Config represents Redis client configuration
type Config struct {
	// Addr is the Redis server address (e.g., "localhost:6379")
	Addr string

	// Addrs is a seed list of host:port addresses, for a Redis Cluster or for
	// the Sentinel nodes of a failover setup. It is read only by
	// [NewUniversalClient]; [NewClient] always builds a single-node client
	// from Addr. When it is empty, NewUniversalClient falls back to Addr.
	Addrs []string

	// MasterName is the Sentinel master name. Setting it makes
	// [NewUniversalClient] return a Sentinel-backed failover client, with
	// Addrs (or Addr) read as the Sentinel addresses rather than as Redis
	// servers.
	MasterName string

	// SentinelUsername and SentinelPassword authenticate to the Sentinel
	// nodes themselves. They are separate from Password, which authenticates
	// to the Redis server Sentinel points at -- the two frequently differ,
	// and a Sentinel deployment with ACLs is unusable without them.
	SentinelUsername string
	SentinelPassword string

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

	// Dialer is optional custom dialer (e.g. for mock in tests). When set, Addr can be a placeholder.
	Dialer Dialer
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

// WithAddrs sets the seed list of cluster or Sentinel addresses. Only
// [NewUniversalClient] reads it.
func (c Config) WithAddrs(addrs ...string) Config {
	c.Addrs = addrs
	return c
}

// WithMasterName sets the Sentinel master name, selecting a failover client in
// [NewUniversalClient].
func (c Config) WithMasterName(name string) Config {
	c.MasterName = name
	return c
}

// WithSentinelAuth sets the credentials used against the Sentinel nodes, which
// are not the credentials used against the Redis server behind them.
func (c Config) WithSentinelAuth(username, password string) Config {
	c.SentinelUsername = username
	c.SentinelPassword = password
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
