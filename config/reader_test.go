package config

import (
	"testing"
)

func TestReadTree(t *testing.T) {
	tree := map[string]any{
		"db": map[string]any{
			"host": "localhost",
		},
	}

	v, ok := readTree(tree, "db.host")
	if !ok {
		t.Fatal("expected to find db.host")
	}
	if v != "localhost" {
		t.Fatalf("expected 'localhost', got %v", v)
	}

	_, ok = readTree(tree, "db.missing")
	if ok {
		t.Fatal("expected not to find db.missing")
	}
}

func TestReader_MergeAndResolve(t *testing.T) {
	opts := defaultOptions()
	r := newReader(opts)

	if err := r.Merge(&KeyValue{Key: "host", Value: []byte("localhost")}); err != nil {
		t.Fatal(err)
	}
	if err := r.Merge(&KeyValue{Key: "addr", Value: []byte("${host}:8080")}); err != nil {
		t.Fatal(err)
	}
	if err := r.Resolve(); err != nil {
		t.Fatal(err)
	}

	v, ok := r.Value("addr")
	if !ok {
		t.Fatal("expected to find addr")
	}
	if v != "localhost:8080" {
		t.Fatalf("expected 'localhost:8080', got %v", v)
	}
}

func TestReader_Source(t *testing.T) {
	opts := defaultOptions()
	r := newReader(opts)
	r.Merge(&KeyValue{Key: "host", Value: []byte("localhost")})

	data, err := r.Source()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"host":"localhost"}` {
		t.Fatalf("unexpected source: %s", data)
	}
}
