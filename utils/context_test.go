package utils

import (
	"context"
	"testing"
	"time"
)

func TestWithTimeout(t *testing.T) {
	t.Run("creates context with timeout", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		if ctx == nil {
			t.Fatal("context should not be nil")
		}

		// Verify timeout works
		select {
		case <-ctx.Done():
			// Good, context was cancelled
		case <-time.After(200 * time.Millisecond):
			t.Fatal("context should have timed out")
		}
	})

	t.Run("handles nil context", func(t *testing.T) {
		// Pass nil explicitly to test the nil check branch
		//nolint:staticcheck // SA1012: intentionally passing nil to test nil handling
		ctx, cancel := WithTimeout(nil, 100*time.Millisecond)
		defer cancel()

		if ctx == nil {
			t.Fatal("context should not be nil (should use Background)")
		}

		// Verify it's a valid context that times out
		select {
		case <-ctx.Done():
			// Good, context was cancelled
		case <-time.After(200 * time.Millisecond):
			t.Fatal("context should have timed out")
		}
	})

	t.Run("handles context.TODO", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.TODO(), 100*time.Millisecond)
		defer cancel()

		if ctx == nil {
			t.Fatal("context should not be nil")
		}

		// Verify it's a valid context
		select {
		case <-ctx.Done():
			// Good
		case <-time.After(200 * time.Millisecond):
			t.Fatal("context should have timed out")
		}
	})

	t.Run("timeout is respected", func(t *testing.T) {
		start := time.Now()
		ctx, cancel := WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		<-ctx.Done()
		elapsed := time.Since(start)

		if elapsed < 50*time.Millisecond {
			t.Errorf("context cancelled too early: %v", elapsed)
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("context cancelled too late: %v", elapsed)
		}
	})

	t.Run("cancel before timeout", func(t *testing.T) {
		start := time.Now()
		ctx, cancel := WithTimeout(context.Background(), 1*time.Second)

		// Cancel immediately
		cancel()

		<-ctx.Done()
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("context should have been cancelled immediately, took %v", elapsed)
		}
	})
}

func TestWithDefaultTimeout(t *testing.T) {
	t.Run("creates context with default timeout", func(t *testing.T) {
		ctx, cancel := WithDefaultTimeout(context.Background())
		defer cancel()

		if ctx == nil {
			t.Fatal("context should not be nil")
		}

		// Verify timeout works (default is 5 seconds)
		select {
		case <-ctx.Done():
			t.Fatal("context should not have timed out immediately")
		case <-time.After(100 * time.Millisecond):
			// Good, context is still valid
		}
	})

	t.Run("handles nil context", func(t *testing.T) {
		// Pass nil explicitly to test the nil check branch
		//nolint:staticcheck // SA1012: intentionally passing nil to test nil handling
		ctx, cancel := WithDefaultTimeout(nil)
		defer cancel()

		if ctx == nil {
			t.Fatal("context should not be nil (should use Background)")
		}

		// Verify it's a valid context
		select {
		case <-ctx.Done():
			t.Fatal("context should not have timed out immediately")
		case <-time.After(100 * time.Millisecond):
			// Good
		}
	})

	t.Run("handles context.TODO", func(t *testing.T) {
		ctx, cancel := WithDefaultTimeout(context.TODO())
		defer cancel()

		if ctx == nil {
			t.Fatal("context should not be nil")
		}

		// Verify it's a valid context
		select {
		case <-ctx.Done():
			t.Fatal("context should not have timed out immediately")
		case <-time.After(100 * time.Millisecond):
			// Good
		}
	})

	t.Run("default timeout is DefaultOperationTimeout", func(t *testing.T) {
		start := time.Now()
		ctx, cancel := WithDefaultTimeout(context.Background())
		defer cancel()

		<-ctx.Done()
		elapsed := time.Since(start)

		// Should be approximately DefaultOperationTimeout (5 seconds)
		expected := DefaultOperationTimeout
		if elapsed < expected-100*time.Millisecond {
			t.Errorf("context cancelled too early: %v, expected ~%v", elapsed, expected)
		}
		if elapsed > expected+100*time.Millisecond {
			t.Errorf("context cancelled too late: %v, expected ~%v", elapsed, expected)
		}
	})

	t.Run("cancel before default timeout", func(t *testing.T) {
		start := time.Now()
		ctx, cancel := WithDefaultTimeout(context.Background())

		// Cancel immediately
		cancel()

		<-ctx.Done()
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("context should have been cancelled immediately, took %v", elapsed)
		}
	})
}
