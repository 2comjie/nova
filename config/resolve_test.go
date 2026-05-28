package config

import (
	"reflect"
	"testing"
)

func TestResolve_SimpleRef(t *testing.T) {
	tree := map[string]any{
		"host": "localhost",
		"addr": "${host}:8080",
	}
	resolve(tree, false)
	if tree["addr"] != "localhost:8080" {
		t.Fatalf("expected 'localhost:8080', got %v", tree["addr"])
	}
}

func TestResolve_NestedRef(t *testing.T) {
	tree := map[string]any{
		"db": map[string]any{
			"host": "localhost",
			"addr": "${db.host}:3306",
		},
	}
	resolve(tree, false)
	addr := tree["db"].(map[string]any)["addr"].(string)
	if addr != "localhost:3306" {
		t.Fatalf("expected 'localhost:3306', got %v", addr)
	}
}

func TestResolve_DefaultValue(t *testing.T) {
	tree := map[string]any{
		"port": "${missing:8080}",
	}
	resolve(tree, false)
	if tree["port"] != "8080" {
		t.Fatalf("expected '8080', got %v", tree["port"])
	}
}

func TestResolve_NoPlaceholder(t *testing.T) {
	tree := map[string]any{
		"host": "localhost",
		"port": int64(8080),
	}
	before := make(map[string]any)
	for k, v := range tree {
		before[k] = v
	}
	resolve(tree, false)
	if !reflect.DeepEqual(tree, before) {
		t.Fatal("tree with no placeholders must remain unchanged")
	}
}

func TestResolve_ToType(t *testing.T) {
	tree := map[string]any{
		"debug": "true",
		"app":   "${debug}",
	}
	resolve(tree, true)

	switch v := tree["app"].(type) {
	case bool:
		if !v {
			t.Fatal("expected true")
		}
	default:
		t.Fatalf("expected bool, got %T", v)
	}
}

func TestResolve_ToTypeKeepsStrings(t *testing.T) {
	tree := map[string]any{
		"host": "localhost",
		"addr": "${host}:8080",
	}
	resolve(tree, true)
	// addr has surrounding text, so it should stay as string replacement
	if tree["addr"] != "localhost:8080" {
		t.Fatalf("expected 'localhost:8080', got %v", tree["addr"])
	}
}

func TestResolve_ArrayElements(t *testing.T) {
	tree := map[string]any{
		"host": "localhost",
		"endpoints": []any{
			"${host}:8080",
			map[string]any{"url": "${host}:9090"},
		},
	}
	resolve(tree, false)
	eps := tree["endpoints"].([]any)
	if eps[0] != "localhost:8080" {
		t.Fatalf("expected 'localhost:8080', got %v", eps[0])
	}
	url := eps[1].(map[string]any)["url"].(string)
	if url != "localhost:9090" {
		t.Fatalf("expected 'localhost:9090', got %v", url)
	}
}
