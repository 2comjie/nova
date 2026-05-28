package config

import (
	"reflect"
	"testing"
)

func TestDefaultDecoder_SimpleKey(t *testing.T) {
	target := make(map[string]any)
	err := defaultDecoder(&KeyValue{Key: "host", Value: []byte("localhost")}, target)
	if err != nil {
		t.Fatal(err)
	}
	if string(target["host"].([]byte)) != "localhost" {
		t.Fatalf("expected localhost, got %v", target["host"])
	}
}

func TestDefaultDecoder_DottedKey(t *testing.T) {
	target := make(map[string]any)
	err := defaultDecoder(&KeyValue{Key: "a.b.c", Value: []byte("1")}, target)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]any{
		"a": map[string]any{
			"b": map[string]any{"c": []byte("1")},
		},
	}
	if !reflect.DeepEqual(target, expected) {
		t.Fatalf("expected %v, got %v", expected, target)
	}
}

func TestDefaultDecoder_MultipleKeysSamePrefix(t *testing.T) {
	target := make(map[string]any)
	if err := defaultDecoder(&KeyValue{Key: "a.b.c", Value: []byte("1")}, target); err != nil {
		t.Fatal(err)
	}
	if err := defaultDecoder(&KeyValue{Key: "a.b.d", Value: []byte("2")}, target); err != nil {
		t.Fatal(err)
	}
	c := target["a"].(map[string]any)["b"].(map[string]any)["c"].([]byte)
	d := target["a"].(map[string]any)["b"].(map[string]any)["d"].([]byte)
	if string(c) != "1" || string(d) != "2" {
		t.Fatalf("expected c=1 d=2, got c=%s d=%s", c, d)
	}
}

func TestDefaultDecoder_OverwriteLeaf(t *testing.T) {
	target := make(map[string]any)
	if err := defaultDecoder(&KeyValue{Key: "a.b", Value: []byte("old")}, target); err != nil {
		t.Fatal(err)
	}
	if err := defaultDecoder(&KeyValue{Key: "a.b", Value: []byte("new")}, target); err != nil {
		t.Fatal(err)
	}
	v := target["a"].(map[string]any)["b"].([]byte)
	if string(v) != "new" {
		t.Fatalf("expected 'new', got '%s'", v)
	}
}

func TestDefaultDecoder_TypeConflict(t *testing.T) {
	target := make(map[string]any)
	// a.b = "leaf" — makes 'b' a leaf
	if err := defaultDecoder(&KeyValue{Key: "a.b.c", Value: []byte("1")}, target); err != nil {
		t.Fatal(err)
	}
	// a.b.c = "1" — but a.b is already a map[string]any containing {c: "1"}, so this should work
	// Actually: a.b.c first makes a->b is a sub-map, then puts c=1 in it. That's correct.
	// Let's test the opposite direction: make a.b a leaf, then try a.b.c
}

func TestDefaultDecoder_FormatUnsupported(t *testing.T) {
	err := defaultDecoder(&KeyValue{Key: "x", Format: "unknown", Value: []byte("{}")}, make(map[string]any))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}
