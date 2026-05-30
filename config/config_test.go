package config

import (
	"context"
	"sync"
	"testing"
	"time"
)

type testSource struct {
	mu      sync.Mutex
	kvs     []*KeyValue
	watcher *testWatcher
}

func newTestSource(kvs ...*KeyValue) *testSource {
	return &testSource{
		kvs:     kvs,
		watcher: newTestWatcher(),
	}
}

func (s *testSource) Load() ([]*KeyValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*KeyValue(nil), s.kvs...), nil
}

func (s *testSource) Watch() (Watcher, error) {
	return s.watcher, nil
}

func (s *testSource) set(kvs ...*KeyValue) {
	s.mu.Lock()
	s.kvs = kvs
	s.mu.Unlock()
}

func (s *testSource) trigger() {
	s.watcher.trigger()
}

type testWatcher struct {
	events chan struct{}
	done   chan struct{}
}

func newTestWatcher() *testWatcher {
	return &testWatcher{
		events: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
}

func (w *testWatcher) Next() ([]*KeyValue, error) {
	select {
	case <-w.events:
		return nil, nil
	case <-w.done:
		return nil, context.Canceled
	}
}

func (w *testWatcher) Stop() {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
}

func (w *testWatcher) trigger() {
	select {
	case w.events <- struct{}{}:
	default:
	}
}

func TestConfigWatchReloadsCachedValue(t *testing.T) {
	src := newTestSource(&KeyValue{Key: "server.port", Value: []byte("8080")})
	c := New(WithSource(src))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	port := c.Value("server.port")
	if port == nil {
		t.Fatal("expected server.port")
	}

	src.set(&KeyValue{Key: "server.port", Value: []byte("9090")})
	src.trigger()

	waitFor(t, func() bool {
		return port.Load() == "9090"
	})
}

func TestConfigWatchClearsRemovedCachedValue(t *testing.T) {
	src := newTestSource(&KeyValue{Key: "server.port", Value: []byte("8080")})
	c := New(WithSource(src))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	port := c.Value("server.port")
	if port == nil {
		t.Fatal("expected server.port")
	}

	src.set(&KeyValue{Key: "server.host", Value: []byte("localhost")})
	src.trigger()

	waitFor(t, func() bool {
		return port.Load() == nil
	})
}

func TestConfigWatchAllowsCachedValueTypeChange(t *testing.T) {
	src := newTestSource(&KeyValue{Key: "server.port", Value: []byte("8080")})
	c := New(WithSource(src), WithResolveActualTypes(true))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	port := c.Value("server.port")
	if port == nil {
		t.Fatal("expected server.port")
	}

	src.set(&KeyValue{Key: "server.port", Value: []byte("disabled")})
	src.trigger()

	waitFor(t, func() bool {
		return port.Load() == "disabled"
	})
}

func TestConfigWatchCanObserveSubtree(t *testing.T) {
	src := newTestSource(&KeyValue{Key: "server.port", Value: []byte("8080")})
	c := New(WithSource(src))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	changed := make(chan Value, 1)
	if err := c.Watch("server", func(_ string, v Value) {
		changed <- v
	}); err != nil {
		t.Fatal(err)
	}

	src.set(&KeyValue{Key: "server.port", Value: []byte("9090")})
	src.trigger()

	select {
	case v := <-changed:
		server, ok := v.Load().(map[string]any)
		if !ok {
			t.Fatalf("got %T, want map[string]any", v.Load())
		}
		if server["port"] != "9090" {
			t.Fatalf("got server.port = %v, want 9090", server["port"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subtree watch")
	}
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()

	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("condition was not met")
		case <-tick.C:
			if fn() {
				return
			}
		}
	}
}
