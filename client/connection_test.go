package client

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

var contextConstructors = map[string]func(context.Context, Config) (redis.UniversalClient, error){
	"standalone": func(ctx context.Context, cfg Config) (redis.UniversalClient, error) {
		rdb, err := NewClientContext(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return rdb, nil
	},
	"universal": NewUniversalClientContext,
}

// connectionServer is a real TCP/TLS RESP2 endpoint. Unlike option-only tests,
// it checks the credentials and transport actually sent by the constructors.
type connectionServer struct {
	listener    net.Listener
	username    string
	password    string
	stallPing   atomic.Bool
	pingSeen    chan struct{}
	mu          sync.Mutex
	connections []net.Conn
	closed      bool
	wg          sync.WaitGroup
}

func newConnectionServer(t *testing.T, tlsConfig *tls.Config, username, password string) *connectionServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	s := &connectionServer{listener: listener, username: username, password: password, pingSeen: make(chan struct{}, 8)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				_ = conn.Close()
				return
			}
			s.connections = append(s.connections, conn)
			s.wg.Add(1)
			s.mu.Unlock()
			go func() { defer s.wg.Done(); defer conn.Close(); s.serve(conn) }()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		s.mu.Lock()
		s.closed = true
		for _, conn := range s.connections {
			_ = conn.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	})
	return s
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || line[0] != '*' {
		return nil, fmt.Errorf("invalid RESP array %q", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, n)
	for i := range args {
		line, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if len(line) < 3 || line[0] != '$' {
			return nil, fmt.Errorf("invalid RESP bulk %q", line)
		}
		length, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return nil, err
		}
		data := make([]byte, length+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		args[i] = string(data[:length])
	}
	return args, nil
}

func (s *connectionServer) serve(conn net.Conn) {
	reader := bufio.NewReader(conn)
	authenticated := s.password == ""
	for {
		args, err := readRESPCommand(reader)
		if err != nil {
			return
		}
		response := "+OK\r\n"
		switch strings.ToUpper(args[0]) {
		case "HELLO":
			response = "-ERR unknown command HELLO\r\n"
		case "AUTH":
			if (len(args) == 2 && s.username == "" && args[1] == s.password) ||
				(len(args) == 3 && args[1] == s.username && args[2] == s.password) {
				authenticated = true
			} else {
				response = "-WRONGPASS invalid username-password pair\r\n"
			}
		case "PING":
			select {
			case s.pingSeen <- struct{}{}:
			default:
			}
			if s.stallPing.Load() {
				continue
			}
			if authenticated {
				response = "+PONG\r\n"
			} else {
				response = "-NOAUTH Authentication required\r\n"
			}
		}
		if _, err := io.WriteString(conn, response); err != nil {
			return
		}
	}
}

func TestConstructorsAuthenticateACLAndDefaultUser(t *testing.T) {
	for shape, constructor := range contextConstructors {
		for _, username := range []string{"", "owlcron"} {
			t.Run(shape+"/"+username, func(t *testing.T) {
				server := newConnectionServer(t, nil, username, "secret")
				cfg := DefaultConfig().WithAddr(server.listener.Addr().String()).WithMinIdleConns(0).
					WithUsername(username).WithPassword("secret").WithMaxRetries(-1).WithContextTimeoutEnabled(true)
				ctx, cancel := context.WithCancel(context.Background())
				rdb, err := constructor(ctx, cfg)
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				defer rdb.Close()
				if err := rdb.Ping(context.Background()).Err(); err != nil {
					t.Fatalf("startup cancellation closed returned client: %v", err)
				}
				opts := rdb.(*redis.Client).Options()
				if opts.Username != username || !opts.ContextTimeoutEnabled || opts.MaxRetries != 0 {
					t.Fatalf("unexpected options: username=%q, context=%v, retries=%d", opts.Username, opts.ContextTimeoutEnabled, opts.MaxRetries)
				}
				// Subsequent pool dials must also be detached from startup.
				conn, err := opts.Dialer(context.Background(), "tcp", cfg.Addr)
				if err != nil {
					t.Fatalf("dial retained canceled startup context: %v", err)
				}
				_ = conn.Close()
				if bad, err := constructor(context.Background(), cfg.WithPassword("wrong")); err == nil {
					_ = bad.Close()
					t.Fatal("wrong password authenticated")
				}
				if username != "" {
					if bad, err := constructor(context.Background(), cfg.WithUsername("wrong")); err == nil {
						_ = bad.Close()
						t.Fatal("wrong ACL username authenticated")
					}
				}
			})
		}
	}
}

func TestConstructorsRespectCanceledContextBeforeDial(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			cfg := DefaultConfig()
			cfg.Dialer = func(context.Context, string, string) (net.Conn, error) {
				calls.Add(1)
				return nil, errors.New("unexpected dial")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			rdb, err := constructor(ctx, cfg)
			if rdb != nil || !errors.Is(err, context.Canceled) || calls.Load() != 0 {
				t.Fatalf("client=%v error=%v dials=%d", rdb, err, calls.Load())
			}
		})
	}
}

func TestConstructorsCancelInFlightDial(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			var once sync.Once
			cfg := DefaultConfig().WithMinIdleConns(0).WithMaxRetries(-1)
			cfg.Dialer = func(ctx context.Context, _, _ string) (net.Conn, error) {
				once.Do(func() { close(started) })
				<-ctx.Done()
				close(stopped)
				return nil, ctx.Err()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := constructor(ctx, cfg); result <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("dial did not start")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("constructor ignored dial cancellation")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("dial goroutine retained after cancellation")
			}
		})
	}
}

func TestConstructorsBoundStalledPING(t *testing.T) {
	for name, constructor := range contextConstructors {
		for _, contextTimeouts := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/context-timeouts=%v", name, contextTimeouts), func(t *testing.T) {
				server := newConnectionServer(t, nil, "", "")
				server.stallPing.Store(true)
				cfg := DefaultConfig().WithAddr(server.listener.Addr().String()).WithMinIdleConns(0).
					WithDialTimeout(60 * time.Millisecond).WithReadTimeout(5 * time.Second).
					WithMaxRetries(-1).WithContextTimeoutEnabled(contextTimeouts)
				start := time.Now()
				_, err := constructor(context.Background(), cfg)
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected startup deadline: %v", err)
				}
				if elapsed := time.Since(start); elapsed > time.Second {
					t.Fatalf("stalled PING ignored startup budget: %s", elapsed)
				}
			})
		}
	}
}

func TestConstructorsRespectCallerDeadlineDuringPING(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			server := newConnectionServer(t, nil, "", "")
			server.stallPing.Store(true)
			cfg := DefaultConfig().WithAddr(server.listener.Addr().String()).WithMinIdleConns(0).
				WithReadTimeout(5 * time.Second).WithMaxRetries(-1)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := constructor(ctx, cfg)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected caller deadline: %v", err)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("stalled PING ignored caller budget: %s", elapsed)
			}
		})
	}
}

func TestConstructorsCancelStalledPING(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			server := newConnectionServer(t, nil, "", "")
			server.stallPing.Store(true)
			cfg := DefaultConfig().WithAddr(server.listener.Addr().String()).WithMinIdleConns(0).
				WithReadTimeout(5 * time.Second).WithMaxRetries(-1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := constructor(ctx, cfg); result <- err }()
			select {
			case <-server.pingSeen:
			case <-time.After(time.Second):
				t.Fatal("PING did not reach server")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("PING ignored cancellation")
			}
		})
	}
}

func TestCommandDeadlineAfterStartup(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			server := newConnectionServer(t, nil, "", "")
			cfg := DefaultConfig().WithAddr(server.listener.Addr().String()).WithMinIdleConns(0).
				WithReadTimeout(5 * time.Second).WithMaxRetries(-1).WithContextTimeoutEnabled(true)
			rdb, err := constructor(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer rdb.Close()
			server.stallPing.Store(true)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
			defer cancel()
			start := time.Now()
			if err := rdb.Ping(ctx).Err(); err == nil {
				t.Fatal("stalled PING succeeded")
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("command ignored context deadline: %s", elapsed)
			}
		})
	}
}

func testTLSConfig(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"redis.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "redis.test"}
}

func TestConstructorsVerifyTLSAndCloneConfiguration(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			serverTLS, callerTLS := testTLSConfig(t)
			server := newConnectionServer(t, serverTLS, "owlcron", "secret")
			cfg := DefaultConfig().WithAddr(server.listener.Addr().String()).WithMinIdleConns(0).
				WithUsername("owlcron").WithPassword("secret").WithTLSConfig(callerTLS).WithMaxRetries(-1)
			callerTLS.ServerName = "changed-by-caller"
			rdb, err := constructor(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer rdb.Close()
			opts := rdb.(*redis.Client).Options()
			if opts.TLSConfig == cfg.TLSConfig || opts.TLSConfig.ServerName != "redis.test" {
				t.Fatal("constructor did not clone TLS configuration")
			}
			cfg.TLSConfig.ServerName = "changed-after-construction"
			conn, err := opts.Dialer(context.Background(), "tcp", cfg.Addr)
			if err != nil {
				t.Fatalf("dialer shared mutable TLS configuration: %v", err)
			}
			_ = conn.Close()
			untrusted := cfg.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: "redis.test"})
			if bad, err := constructor(context.Background(), untrusted); err == nil {
				_ = bad.Close()
				t.Fatal("untrusted certificate accepted")
			}
		})
	}
}

func TestConstructorsCancelTLSHandshake(t *testing.T) {
	for name, constructor := range contextConstructors {
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					accepted <- conn
				}
			}()
			cfg := DefaultConfig().WithAddr(listener.Addr().String()).WithMinIdleConns(0).
				WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: "redis.test"}).WithMaxRetries(-1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := constructor(ctx, cfg); result <- err }()
			select {
			case conn := <-accepted:
				defer conn.Close()
			case <-time.After(time.Second):
				t.Fatal("TLS connection not accepted")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("TLS handshake ignored cancellation")
			}
		})
	}
}
