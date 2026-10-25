package client

import (
	"context"
	"crypto/tls"
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
	// nodes themselves. They are separate from Username/Password, which authenticate
	// to the Redis server Sentinel points at -- the two frequently differ,
	// and a Sentinel deployment with ACLs is unusable without them.
	SentinelUsername string
	SentinelPassword string

	// Username is the Redis ACL username. An empty value uses the default user.
	Username string

	// Password is the Redis password (empty if no password)
	Password string

	// TLSConfig enables TLS for Redis connections. Constructors clone this
	// configuration before passing it to go-redis. A custom Dialer owns its
	// transport, including TLS, when provided.
	TLSConfig *tls.Config

	// ContextTimeoutEnabled makes go-redis apply command context deadlines to
	// socket reads and writes. It defaults to false for compatibility. The
	// context-aware constructors always honor their startup context regardless
	// of this setting; it controls subsequent commands on the returned client.
	ContextTimeoutEnabled bool

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

// WithUsername sets the Redis ACL username (empty selects the default user).
func (c Config) WithUsername(username string) Config {
	c.Username = username
	return c
}

// WithTLSConfig enables TLS. A nil configuration disables TLS. The value is
// cloned here and again by constructors to isolate top-level configuration changes.
func (c Config) WithTLSConfig(config *tls.Config) Config {
	if config == nil {
		c.TLSConfig = nil
	} else {
		c.TLSConfig = config.Clone()
	}
	return c
}

// WithContextTimeoutEnabled controls whether command deadlines bound socket I/O.
func (c Config) WithContextTimeoutEnabled(enabled bool) Config {
	c.ContextTimeoutEnabled = enabled
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

// WithMaxRetries sets the maximum number of retries. Use -1 to disable retries;
// go-redis interprets zero as its default, not as "no retries".
func (c Config) WithMaxRetries(retries int) Config {
	c.MaxRetries = retries
	return c
}

// WithPoolTimeout sets the pool timeout
func (c Config) WithPoolTimeout(timeout time.Duration) Config {
	c.PoolTimeout = timeout
	return c
}
