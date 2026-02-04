package testutil

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// MockRedis is a simple in-memory Redis mock for testing
type MockRedis struct {
	data       map[string]mockValue
	mu         sync.RWMutex
	shouldFail bool // For testing error scenarios
}

type mockValue struct {
	value     string
	expiresAt *time.Time
}

// NewMockRedis creates a new mock Redis instance
func NewMockRedis() *MockRedis {
	return &MockRedis{
		data: make(map[string]mockValue),
	}
}

// SetShouldFail sets whether operations should fail
func (m *MockRedis) SetShouldFail(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
}

// dialer creates a connection to the mock Redis
func (m *MockRedis) dialer(_ context.Context, _, _ string) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	go m.serveConn(serverConn)
	return clientConn, nil
}

// Dialer returns a function that can be used as client.Config.Dialer for testing.
func (m *MockRedis) Dialer() func(context.Context, string, string) (net.Conn, error) {
	return m.dialer
}

// serveConn handles connections to the mock Redis
func (m *MockRedis) serveConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	for {
		args, err := readCommand(reader)
		if err != nil {
			return
		}
		if err := m.handleCommand(args, writer); err != nil {
			_ = writer.Flush() // flush error response before closing
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

// handleCommand processes Redis commands
func (m *MockRedis) handleCommand(args []string, w *bufio.Writer) error {
	if len(args) == 0 {
		return writeError(w, "empty command")
	}

	// Check if we should fail
	m.mu.RLock()
	shouldFail := m.shouldFail
	m.mu.RUnlock()
	if shouldFail {
		return writeError(w, "mock redis failure")
	}

	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "PING":
		return writeSimpleString(w, "PONG")
	case "SET":
		return m.handleSet(args, w)
	case "GET":
		return m.handleGet(args, w)
	case "DEL":
		return m.handleDel(args, w)
	case "EXISTS":
		return m.handleExists(args, w)
	case "INCR":
		return m.handleIncr(args, w)
	case "TTL":
		return m.handleTTL(args, w)
	case "EXPIRE":
		return m.handleExpire(args, w)
	case "EVAL":
		return m.handleEval(args, w)
	case "FLUSHDB":
		m.mu.Lock()
		m.data = make(map[string]mockValue)
		m.mu.Unlock()
		return writeSimpleString(w, "OK")
	default:
		return writeError(w, fmt.Sprintf("unknown command: %s", cmd))
	}
}

func (m *MockRedis) handleSet(args []string, w *bufio.Writer) error {
	if len(args) < 3 {
		return writeError(w, "invalid args")
	}

	key := args[1]
	value := args[2]
	ttl := time.Duration(0)
	nx := false

	// Parse options (SET key value [EX seconds|PX milliseconds] [NX|XX])
	for i := 3; i < len(args); i++ {
		opt := strings.ToUpper(args[i])
		if opt == "EX" && i+1 < len(args) {
			seconds, _ := strconv.Atoi(args[i+1])
			ttl = time.Duration(seconds) * time.Second
			i++ // Skip the next argument
		} else if opt == "PX" && i+1 < len(args) {
			millis, _ := strconv.Atoi(args[i+1])
			ttl = time.Duration(millis) * time.Millisecond
			i++ // Skip the next argument
		} else if opt == "NX" {
			nx = true
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if key exists and not expired
	val, exists := m.data[key]
	if exists && val.expiresAt != nil && time.Now().After(*val.expiresAt) {
		// Key expired, treat as not existing
		delete(m.data, key)
		exists = false
	}

	if nx {
		// NX option: only set if key doesn't exist
		if exists {
			// Key exists, return nil (go-redis SetNX interprets this as false)
			return writeNil(w)
		}
		// Key doesn't exist, set it
		var expiresAt *time.Time
		if ttl > 0 {
			exp := time.Now().Add(ttl)
			expiresAt = &exp
		}
		m.data[key] = mockValue{value: value, expiresAt: expiresAt}
		// For SET NX, return OK when successful
		return writeSimpleString(w, "OK")
	}

	var expiresAt *time.Time
	if ttl > 0 {
		exp := time.Now().Add(ttl)
		expiresAt = &exp
	}
	m.data[key] = mockValue{value: value, expiresAt: expiresAt}

	return writeSimpleString(w, "OK")
}

func (m *MockRedis) handleGet(args []string, w *bufio.Writer) error {
	if len(args) < 2 {
		return writeError(w, "invalid args")
	}

	key := args[1]
	m.mu.RLock()
	val, ok := m.data[key]
	m.mu.RUnlock()

	if !ok {
		return writeNil(w)
	}

	// Check expiration
	if val.expiresAt != nil && time.Now().After(*val.expiresAt) {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return writeNil(w)
	}

	return writeBulkString(w, val.value)
}

func (m *MockRedis) handleDel(args []string, w *bufio.Writer) error {
	if len(args) < 2 {
		return writeError(w, "invalid args")
	}

	count := 0
	m.mu.Lock()
	for i := 1; i < len(args); i++ {
		if _, ok := m.data[args[i]]; ok {
			delete(m.data, args[i])
			count++
		}
	}
	m.mu.Unlock()

	return writeInt(w, int64(count))
}

func (m *MockRedis) handleExists(args []string, w *bufio.Writer) error {
	if len(args) < 2 {
		return writeError(w, "invalid args")
	}

	count := 0
	m.mu.RLock()
	for i := 1; i < len(args); i++ {
		val, ok := m.data[args[i]]
		if ok {
			// Check expiration
			if val.expiresAt == nil || time.Now().Before(*val.expiresAt) {
				count++
			}
		}
	}
	m.mu.RUnlock()

	return writeInt(w, int64(count))
}

func (m *MockRedis) handleIncr(args []string, w *bufio.Writer) error {
	if len(args) < 2 {
		return writeError(w, "invalid args")
	}

	key := args[1]
	m.mu.Lock()
	defer m.mu.Unlock()

	val, ok := m.data[key]
	var num int64
	if ok {
		var err error
		num, err = strconv.ParseInt(val.value, 10, 64)
		if err != nil {
			return writeError(w, "value is not an integer")
		}
	} else {
		num = 0
	}

	num++
	m.data[key] = mockValue{value: strconv.FormatInt(num, 10), expiresAt: val.expiresAt}
	return writeInt(w, num)
}

func (m *MockRedis) handleTTL(args []string, w *bufio.Writer) error {
	if len(args) < 2 {
		return writeError(w, "invalid args")
	}

	key := args[1]
	m.mu.RLock()
	val, ok := m.data[key]
	m.mu.RUnlock()

	if !ok {
		return writeInt(w, -2) // Key doesn't exist
	}

	if val.expiresAt == nil {
		return writeInt(w, -1) // No expiration
	}

	ttl := time.Until(*val.expiresAt)
	if ttl <= 0 {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return writeInt(w, -2) // Key expired
	}

	return writeInt(w, int64(ttl.Seconds()))
}

func (m *MockRedis) handleExpire(args []string, w *bufio.Writer) error {
	if len(args) < 3 {
		return writeError(w, "invalid args")
	}

	key := args[1]
	seconds, err := strconv.Atoi(args[2])
	if err != nil {
		return writeError(w, "invalid seconds")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	val, ok := m.data[key]
	if !ok {
		return writeInt(w, 0)
	}

	exp := time.Now().Add(time.Duration(seconds) * time.Second)
	val.expiresAt = &exp
	m.data[key] = val

	return writeInt(w, 1)
}

func (m *MockRedis) handleEval(args []string, w *bufio.Writer) error {
	if len(args) < 3 {
		return writeError(w, "invalid args")
	}

	// Simple Lua script support for lock unlock
	script := args[1]
	numKeys, err := strconv.Atoi(args[2])
	if err != nil {
		return writeError(w, "invalid numkeys")
	}

	if numKeys < 1 || len(args) < 3+numKeys+1 {
		return writeError(w, "invalid args")
	}

	key := args[3]
	argv := args[3+numKeys:]

	// Handle the unlock script: if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end
	if strings.Contains(script, "get") && strings.Contains(script, "del") {
		m.mu.Lock()
		defer m.mu.Unlock()

		if len(argv) < 1 {
			return writeError(w, "invalid args")
		}
		lockValue := argv[0]
		val, ok := m.data[key]
		if !ok {
			return writeInt(w, 0)
		}

		if val.value == lockValue {
			delete(m.data, key)
			return writeInt(w, 1)
		}

		return writeInt(w, 0)
	}

	if strings.Contains(script, "redis-kit:ratelimit") {
		if len(argv) < 2 {
			return writeError(w, "invalid args")
		}
		limit, err := strconv.ParseInt(argv[0], 10, 64)
		if err != nil {
			return writeError(w, "invalid limit")
		}
		windowMs, err := strconv.ParseInt(argv[1], 10, 64)
		if err != nil {
			return writeError(w, "invalid window")
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		val, ok := m.data[key]
		if ok && val.expiresAt != nil && time.Now().After(*val.expiresAt) {
			delete(m.data, key)
			ok = false
		}

		if !ok {
			exp := time.Now().Add(time.Duration(windowMs) * time.Millisecond)
			m.data[key] = mockValue{value: "1", expiresAt: &exp}
			remaining := limit - 1
			if remaining < 0 {
				remaining = 0
			}
			return writeArrayInt(w, []int64{1, remaining, windowMs})
		}

		current, err := strconv.ParseInt(val.value, 10, 64)
		if err != nil {
			return writeError(w, "value is not an integer")
		}
		if current >= limit {
			ttl := ttlMilliseconds(val.expiresAt)
			return writeArrayInt(w, []int64{0, 0, ttl})
		}

		current++
		if val.expiresAt == nil {
			exp := time.Now().Add(time.Duration(windowMs) * time.Millisecond)
			val.expiresAt = &exp
		}
		val.value = strconv.FormatInt(current, 10)
		m.data[key] = val
		remaining := limit - current
		if remaining < 0 {
			remaining = 0
		}
		ttl := ttlMilliseconds(val.expiresAt)
		return writeArrayInt(w, []int64{1, remaining, ttl})
	}

	if strings.Contains(script, "redis-kit:cooldown") {
		if len(argv) < 1 {
			return writeError(w, "invalid args")
		}
		cooldownMs, err := strconv.ParseInt(argv[0], 10, 64)
		if err != nil {
			return writeError(w, "invalid cooldown")
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		val, ok := m.data[key]
		if ok && val.expiresAt != nil && time.Now().After(*val.expiresAt) {
			delete(m.data, key)
			ok = false
		}

		if !ok {
			exp := time.Now().Add(time.Duration(cooldownMs) * time.Millisecond)
			m.data[key] = mockValue{value: "1", expiresAt: &exp}
			return writeArrayInt(w, []int64{1, cooldownMs})
		}

		ttl := ttlMilliseconds(val.expiresAt)
		return writeArrayInt(w, []int64{0, ttl})
	}

	return writeError(w, "unsupported script")
}

// NewMockRedisClient creates a Redis client that uses the mock
func NewMockRedisClient() (*redis.Client, *MockRedis) {
	mock := NewMockRedis()
	client := redis.NewClient(&redis.Options{
		Addr:   "mock",
		Dialer: mock.dialer,
	})
	return client, mock
}

// Helper functions for RESP protocol

func readCommand(r *bufio.Reader) ([]string, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if prefix != '*' {
		return nil, errors.New("unexpected RESP prefix")
	}

	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(line)
	if err != nil {
		return nil, err
	}

	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		bulkPrefix, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if bulkPrefix != '$' {
			return nil, errors.New("unexpected bulk prefix")
		}
		lenLine, err := readLine(r)
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(lenLine)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}

	return args, nil
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

func writeSimpleString(w *bufio.Writer, msg string) error {
	_, err := w.WriteString("+" + msg + "\r\n")
	return err
}

func writeError(w *bufio.Writer, msg string) error {
	_, err := w.WriteString("-ERR " + msg + "\r\n")
	return err
}

func writeInt(w *bufio.Writer, value int64) error {
	_, err := w.WriteString(":" + strconv.FormatInt(value, 10) + "\r\n")
	return err
}

func writeBulkString(w *bufio.Writer, value string) error {
	_, err := w.WriteString("$" + strconv.Itoa(len(value)) + "\r\n" + value + "\r\n")
	return err
}

func writeNil(w *bufio.Writer) error {
	_, err := w.WriteString("$-1\r\n")
	return err
}

func writeArrayInt(w *bufio.Writer, values []int64) error {
	if _, err := w.WriteString("*" + strconv.Itoa(len(values)) + "\r\n"); err != nil {
		return err
	}
	for _, value := range values {
		if err := writeInt(w, value); err != nil {
			return err
		}
	}
	return nil
}

func ttlMilliseconds(expiresAt *time.Time) int64 {
	if expiresAt == nil {
		return -1
	}
	ttl := time.Until(*expiresAt)
	if ttl <= 0 {
		return -2
	}
	return int64(ttl / time.Millisecond)
}
