package config

import (
	"reflect"
	"testing"
)

func TestMerge_Flat(t *testing.T) {
	dst := map[string]any{"a": int64(1)}
	src := map[string]any{"b": int64(2)}
	merge(dst, src)
	expected := map[string]any{"a": int64(1), "b": int64(2)}
	if !reflect.DeepEqual(dst, expected) {
		t.Fatalf("expected %v, got %v", expected, dst)
	}
}

func TestMerge_Overwrite(t *testing.T) {
	dst := map[string]any{"a": int64(1)}
	src := map[string]any{"a": int64(2)}
	merge(dst, src)
	if dst["a"] != int64(2) {
		t.Fatalf("expected 2, got %v", dst["a"])
	}
}

func TestMerge_Nested(t *testing.T) {
	dst := map[string]any{
		"db": map[string]any{"host": "localhost"},
	}
	src := map[string]any{
		"db": map[string]any{"port": int64(3306)},
	}
	merge(dst, src)
	expected := map[string]any{
		"db": map[string]any{"host": "localhost", "port": int64(3306)},
	}
	if !reflect.DeepEqual(dst, expected) {
		t.Fatalf("expected %v, got %v", expected, dst)
	}
}

func TestMerge_NestedOverwrite(t *testing.T) {
	dst := map[string]any{
		"db": map[string]any{"host": "localhost", "port": int64(3306)},
	}
	src := map[string]any{
		"db": map[string]any{"host": "0.0.0.0"},
	}
	merge(dst, src)
	expected := map[string]any{
		"db": map[string]any{"host": "0.0.0.0", "port": int64(3306)},
	}
	if !reflect.DeepEqual(dst, expected) {
		t.Fatalf("expected %v, got %v", expected, dst)
	}
}

func TestMerge_NewNested(t *testing.T) {
	dst := map[string]any{}
	src := map[string]any{
		"a": map[string]any{"b": map[string]any{"c": int64(1)}},
	}
	merge(dst, src)
	if !reflect.DeepEqual(dst, src) {
		t.Fatalf("expected %v, got %v", src, dst)
	}
}

func TestMerge_CloneSemantics(t *testing.T) {
	src := map[string]any{
		"items": []any{int64(1), int64(2)},
	}
	dst := map[string]any{}
	merge(dst, src)
	// mutate src, verify dst is independent
	src["items"].([]any)[0] = int64(999)
	if dst["items"].([]any)[0] != int64(1) {
		t.Fatal("merged value must be independent from source")
	}
}
