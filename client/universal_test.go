package client

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soulteary/redis-kit/testutil"
)

// newMockDialerConfig points a Config at the in-memory mock, so a constructor
// can complete its connection test without a Redis server.
func newMockDialerConfig() Config {
	_, mock := testutil.NewMockRedisClient()
	cfg := DefaultConfig().WithAddr("mock:6379").WithDialTimeout(2 * time.Second)
	cfg.Dialer = mock.Dialer()
	return cfg
}

// The package's functions take a redis.UniversalClient, so every go-redis
// client shape works. Sentinel needs no entry of its own:
// redis.NewFailoverClient returns a *redis.Client and
// redis.NewFailoverClusterClient a *redis.ClusterClient.
var (
	_ redis.UniversalClient = (*redis.Client)(nil)
	_ redis.UniversalClient = (*redis.ClusterClient)(nil)
	_ redis.UniversalClient = (*redis.Ring)(nil)
)

// clientShapes are the typed nils a caller hands over by accident -- an
// unassigned field, or a constructor that returned early. Each is a NON-nil
// interface holding a nil pointer, which a client == nil guard waves through
// to a nil dereference.
func clientShapes() map[string]redis.UniversalClient {
	var (
		standalone *redis.Client
		cluster    *redis.ClusterClient
		ring       *redis.Ring
	)
	return map[string]redis.UniversalClient{
		"nil interface":        nil,
		"*redis.Client":        standalone,
		"*redis.ClusterClient": cluster,
		"*redis.Ring":          ring,
	}
}

// TestNilClientShapesAreReportedNotDereferenced compiles only because each
// function accepts every shape, and passes only because a typed nil is
// reported. A panic here is the regression.
func TestNilClientShapesAreReportedNotDereferenced(t *testing.T) {
	for name, client := range clientShapes() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			const want = "redis client is nil"
			if err := Ping(ctx, client); err == nil || err.Error() != want {
				t.Errorf("Ping() error = %v, want %q", err, want)
			}
			if err := Close(client); err != nil {
				t.Errorf("Close() error = %v, want nil", err)
			}
			if HealthCheck(ctx, client) {
				t.Error("HealthCheck() = true, want false")
			}
			status := CheckHealth(ctx, client)
			if status.Healthy {
				t.Error("CheckHealth().Healthy = true, want false")
			}
			if status.Error == nil || status.Error.Error() != want {
				t.Errorf("CheckHealth().Error = %v, want %q", status.Error, want)
			}
		})
	}
}

func TestNewUniversalClient(t *testing.T) {
	t.Run("rejects a config with no address at all", func(t *testing.T) {
		_, err := NewUniversalClient(Config{})
		if err == nil {
			t.Fatal("NewUniversalClient() with no address should return an error")
		}
		if err.Error() != "redis address is required" {
			t.Errorf("NewUniversalClient() error = %q, want %q", err.Error(), "redis address is required")
		}
	})

	t.Run("falls back to Addr when Addrs is empty", func(t *testing.T) {
		// The mock speaks plain RESP on a pipe, so a single address keeps
		// NewUniversalClient on its single-node branch and PING is answered.
		mock := newMockDialerConfig()
		client, err := NewUniversalClient(mock)
		if err != nil {
			t.Fatalf("NewUniversalClient() error = %v, want nil", err)
		}
		defer func() { _ = client.Close() }()

		if _, ok := client.(*redis.Client); !ok {
			t.Errorf("NewUniversalClient() = %T, want *redis.Client for a single address", client)
		}
		if err := Ping(context.Background(), client); err != nil {
			t.Errorf("Ping() error = %v, want nil", err)
		}
	})

	t.Run("returns a failover client when MasterName is set", func(t *testing.T) {
		// Sentinel selection happens before any dial, so this asserts the
		// routing rule without needing a Sentinel to talk to. The client is
		// discarded unconnected.
		cfg := DefaultConfig().WithAddrs("sentinel-a:26379", "sentinel-b:26379").
			WithMasterName("mymaster").
			WithSentinelAuth("sentinel-user", "sentinel-pass")

		if cfg.MasterName != "mymaster" {
			t.Errorf("WithMasterName() = %q, want %q", cfg.MasterName, "mymaster")
		}
		if len(cfg.Addrs) != 2 {
			t.Errorf("WithAddrs() length = %d, want 2", len(cfg.Addrs))
		}
		if cfg.SentinelUsername != "sentinel-user" || cfg.SentinelPassword != "sentinel-pass" {
			t.Errorf("WithSentinelAuth() = %q/%q, want %q/%q",
				cfg.SentinelUsername, cfg.SentinelPassword, "sentinel-user", "sentinel-pass")
		}

		// Connecting is expected to fail: there is no Sentinel here. What
		// matters is that it fails while dialling rather than at selection.
		cfg = cfg.WithDialTimeout(50 * time.Millisecond)
		if _, err := NewUniversalClient(cfg); err == nil {
			t.Error("NewUniversalClient() against a nonexistent Sentinel should return an error")
		}
	})
}

// A zero DialTimeout used to produce an already-expired context, so the
// connection test failed with "context deadline exceeded" before a packet was
// sent. Both constructors now fall back to the documented default.
func TestDialTimeoutFallsBackToDefault(t *testing.T) {
	if got := dialTimeout(Config{}); got != DefaultConfig().DialTimeout {
		t.Errorf("dialTimeout(zero) = %v, want %v", got, DefaultConfig().DialTimeout)
	}
	if got := dialTimeout(Config{DialTimeout: -time.Second}); got != DefaultConfig().DialTimeout {
		t.Errorf("dialTimeout(negative) = %v, want %v", got, DefaultConfig().DialTimeout)
	}
	if got := dialTimeout(Config{DialTimeout: time.Second}); got != time.Second {
		t.Errorf("dialTimeout(1s) = %v, want %v", got, time.Second)
	}
}
