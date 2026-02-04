package ratelimit

import (
	"testing"
)

// TestToInt64 测试未导出的 toInt64 各分支以提升覆盖率
func TestToInt64(t *testing.T) {
	t.Run("int64", func(t *testing.T) {
		v, ok := toInt64(int64(42))
		if !ok || v != 42 {
			t.Errorf("toInt64(int64(42)) = %d, %v, want 42, true", v, ok)
		}
	})
	t.Run("int", func(t *testing.T) {
		v, ok := toInt64(100)
		if !ok || v != 100 {
			t.Errorf("toInt64(100) = %d, %v, want 100, true", v, ok)
		}
	})
	t.Run("string valid", func(t *testing.T) {
		v, ok := toInt64("999")
		if !ok || v != 999 {
			t.Errorf("toInt64(\"999\") = %d, %v, want 999, true", v, ok)
		}
	})
	t.Run("string invalid", func(t *testing.T) {
		_, ok := toInt64("not-a-number")
		if ok {
			t.Error("toInt64(\"not-a-number\") ok = true, want false")
		}
	})
	t.Run("string empty", func(t *testing.T) {
		_, ok := toInt64("")
		if ok {
			t.Error("toInt64(\"\") ok = true, want false")
		}
	})
	t.Run("default type", func(t *testing.T) {
		_, ok := toInt64([]int{1})
		if ok {
			t.Error("toInt64(slice) ok = true, want false")
		}
		_, ok = toInt64(nil)
		if ok {
			t.Error("toInt64(nil) ok = true, want false")
		}
		_, ok = toInt64(true)
		if ok {
			t.Error("toInt64(bool) ok = true, want false")
		}
	})
}
