package utils

import (
	"testing"
)

func TestBuildKey(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		prefix := "test:"
		key := "mykey"
		result := BuildKey(prefix, key)
		expected := "test:mykey"
		if result != expected {
			t.Errorf("BuildKey(%q, %q) = %q, want %q", prefix, key, result, expected)
		}
	})

	t.Run("without prefix (empty string)", func(t *testing.T) {
		prefix := ""
		key := "mykey"
		result := BuildKey(prefix, key)
		expected := "mykey"
		if result != expected {
			t.Errorf("BuildKey(%q, %q) = %q, want %q", prefix, key, result, expected)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		prefix := "test:"
		key := ""
		result := BuildKey(prefix, key)
		expected := "test:"
		if result != expected {
			t.Errorf("BuildKey(%q, %q) = %q, want %q", prefix, key, result, expected)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		prefix := ""
		key := ""
		result := BuildKey(prefix, key)
		expected := ""
		if result != expected {
			t.Errorf("BuildKey(%q, %q) = %q, want %q", prefix, key, result, expected)
		}
	})
}

func TestBuildKeys(t *testing.T) {
	t.Run("multiple keys with prefix", func(t *testing.T) {
		prefix := "test:"
		keys := []string{"key1", "key2", "key3"}
		result := BuildKeys(prefix, keys...)
		expected := []string{"test:key1", "test:key2", "test:key3"}
		if len(result) != len(expected) {
			t.Fatalf("BuildKeys returned %d keys, want %d", len(result), len(expected))
		}
		for i, r := range result {
			if r != expected[i] {
				t.Errorf("BuildKeys[%d] = %q, want %q", i, r, expected[i])
			}
		}
	})

	t.Run("empty key list", func(t *testing.T) {
		prefix := "test:"
		result := BuildKeys(prefix)
		if len(result) != 0 {
			t.Errorf("BuildKeys with no keys should return empty slice, got %d keys", len(result))
		}
	})

	t.Run("without prefix", func(t *testing.T) {
		prefix := ""
		keys := []string{"key1", "key2"}
		result := BuildKeys(prefix, keys...)
		expected := []string{"key1", "key2"}
		if len(result) != len(expected) {
			t.Fatalf("BuildKeys returned %d keys, want %d", len(result), len(expected))
		}
		for i, r := range result {
			if r != expected[i] {
				t.Errorf("BuildKeys[%d] = %q, want %q", i, r, expected[i])
			}
		}
	})

	t.Run("single key", func(t *testing.T) {
		prefix := "test:"
		keys := []string{"key1"}
		result := BuildKeys(prefix, keys...)
		expected := []string{"test:key1"}
		if len(result) != 1 || result[0] != expected[0] {
			t.Errorf("BuildKeys returned %v, want %v", result, expected)
		}
	})
}
