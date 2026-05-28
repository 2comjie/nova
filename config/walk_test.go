package config

import (
	"reflect"
	"testing"
)

func TestWalk(t *testing.T) {
	t.Run("leaf values", func(t *testing.T) {
		result := walk("hello", func(x any) any { return x.(string) + " world" })
		if result != "hello world" {
			t.Fatalf("expected 'hello world', got %v", result)
		}
	})

	t.Run("nested map", func(t *testing.T) {
		input := map[string]any{
			"a": map[string]any{"b": int64(1)},
			"c": int64(2),
		}
		result := walk(input, func(x any) any {
			if n, ok := x.(int64); ok {
				return n + 10
			}
			return x
		})
		expected := map[string]any{
			"a": map[string]any{"b": int64(11)},
			"c": int64(12),
		}
		if !reflect.DeepEqual(result, expected) {
			t.Fatalf("expected %v, got %v", expected, result)
		}
	})

	t.Run("slice traversal", func(t *testing.T) {
		input := []any{"a", int64(1), []any{"b", int64(2)}}
		result := walk(input, func(x any) any {
			if n, ok := x.(int64); ok {
				return n * 2
			}
			return x
		})
		expected := []any{"a", int64(2), []any{"b", int64(4)}}
		if !reflect.DeepEqual(result, expected) {
			t.Fatalf("expected %v, got %v", expected, result)
		}
	})

	t.Run("map[any]any key conversion", func(t *testing.T) {
		input := map[any]any{123: map[any]any{"x": int64(1)}}
		result := walk(input, func(x any) any { return x })
		expected := map[string]any{"123": map[string]any{"x": int64(1)}}
		if !reflect.DeepEqual(result, expected) {
			t.Fatalf("expected %v, got %v", expected, result)
		}
	})
}

func TestClone(t *testing.T) {
	original := map[string]any{
		"a": map[string]any{"b": int64(1)},
		"c": []any{int64(2), "d"},
	}
	cloned := clone(original).(map[string]any)

	if !reflect.DeepEqual(original, cloned) {
		t.Fatal("clone must be deeply equal to original")
	}

	// mutate clone, verify original unchanged
	cloned["a"].(map[string]any)["b"] = int64(99)
	cloned["c"].([]any)[0] = int64(0)
	if original["a"].(map[string]any)["b"] != int64(1) {
		t.Fatal("original must not be affected by clone mutation")
	}
}

func TestNormalize(t *testing.T) {
	input := map[string]any{
		"host": []byte("localhost"),
		"db":   map[any]any{"host": []byte("dbhost")},
	}
	result := normalize(input).(map[string]any)

	if s, ok := result["host"].(string); !ok || s != "localhost" {
		t.Fatalf("expected string 'localhost', got %T %v", result["host"], result["host"])
	}
	db := result["db"].(map[string]any)
	if s, ok := db["host"].(string); !ok || s != "dbhost" {
		t.Fatalf("expected string 'dbhost', got %T %v", db["host"], db["host"])
	}
}
