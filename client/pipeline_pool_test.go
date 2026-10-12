package client

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/soulteary/redis-kit/testutil"
)

// A normal command establishes the main-pool connection during construction.
// A pipeline must reuse it instead of creating the extra pool introduced in
// go-redis v9.23.0. Count actual dials, rather than merely checking an option.
func TestConstructorsSharePipelineConnection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(Config) (redis.UniversalClient, error)
	}{
		{"standalone", func(cfg Config) (redis.UniversalClient, error) { return NewClient(cfg) }},
		{"universal", NewUniversalClient},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockClient, mock := testutil.NewMockRedisClient()
			t.Cleanup(func() { _ = mockClient.Close() })
			dial := mock.Dialer()
			var dials atomic.Int64
			cfg := DefaultConfig().WithAddr("mock").WithPoolSize(1)
			cfg.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
				dials.Add(1)
				return dial(ctx, network, addr)
			}
			c, err := tc.build(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.Close() })
			for i := 0; i < 3; i++ {
				pipeline := c.Pipeline()
				pipeline.Ping(context.Background())
				if _, err := pipeline.Exec(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if got := dials.Load(); got != 1 {
				t.Fatalf("normal commands and pipelines opened %d connections; want one shared connection", got)
			}
		})
	}
}
