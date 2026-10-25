package client

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"
)

func cloneTLSConfig(config *tls.Config) *tls.Config {
	if config == nil {
		return nil
	}
	return config.Clone()
}

// startupDialer bounds pool background dials by the constructor's context.
// go-redis may otherwise dial on a detached background context. After detach,
// new dials use only their own context and do not retain the startup context.
func startupDialer(startup context.Context, cfg Config) (Dialer, func()) {
	dial := cfg.Dialer
	if dial == nil {
		timeout := cfg.DialTimeout
		if timeout == 0 {
			timeout = DefaultConfig().DialTimeout
		}
		networkDialer := &net.Dialer{
			Timeout: timeout,
			// Preserve go-redis's default TCP keep-alive policy.
			KeepAliveConfig: net.KeepAliveConfig{
				Enable: true, Idle: 30 * time.Second, Interval: 5 * time.Second, Count: 3,
			},
		}
		if cfg.TLSConfig == nil {
			dial = networkDialer.DialContext
		} else {
			// go-redis's TLS default uses tls.DialWithDialer, which does
			// not accept the supplied context. DialContext also bounds TLS
			// negotiation by the constructor or pool dial context.
			tlsDialer := &tls.Dialer{NetDialer: networkDialer, Config: cfg.TLSConfig.Clone()}
			dial = tlsDialer.DialContext
		}
	}
	var mu sync.RWMutex
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
			mu.RLock()
			initial := startup
			mu.RUnlock()
			if initial == nil {
				return dial(ctx, network, addr)
			}
			joined, cancel := context.WithCancel(ctx)
			defer cancel()
			if deadline, ok := initial.Deadline(); ok {
				var cancelDeadline context.CancelFunc
				joined, cancelDeadline = context.WithDeadline(joined, deadline)
				defer cancelDeadline()
			}
			stop := context.AfterFunc(initial, cancel)
			defer stop()
			if err := initial.Err(); err != nil {
				return nil, err
			}
			return dial(joined, network, addr)
		}, func() {
			mu.Lock()
			startup = nil
			mu.Unlock()
		}
}
