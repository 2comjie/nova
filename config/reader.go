package config

import (
	"encoding/json"
	"strings"
	"sync"
)

type Reader struct {
	values map[string]any
	opts   options
	mu     sync.RWMutex
}

func newReader(opts options) *Reader {
	return &Reader{
		values: make(map[string]any),
		opts:   opts,
	}
}

func (r *Reader) Merge(kvs ...*KeyValue) error {
	current := r.snapshot()
	for _, kv := range kvs {
		next := make(map[string]any)
		if err := r.opts.decoder(kv, next); err != nil {
			return err
		}
		r.opts.mergeFunc(current, normalize(next).(map[string]any))
	}
	r.mu.Lock()
	r.values = current
	r.mu.Unlock()
	return nil
}

func (r *Reader) Resolve() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.opts.resolver(r.values)
}

func (r *Reader) Value(path string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return readTree(r.values, path)
}

func (r *Reader) Source() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return json.Marshal(r.values)
}

func (r *Reader) snapshot() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return clone(r.values).(map[string]any)
}

func (r *Reader) readPrimitive(path string) (string, bool) {
	v, ok := r.Value(path)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// readValue returns a Value by dot-separated path.
func readValue(tree map[string]any, path string) (Value, bool) {
	keys := strings.Split(path, ".")
	last := len(keys) - 1
	cur := tree
	for i, key := range keys {
		v, ok := cur[key]
		if !ok {
			return nil, false
		}
		if i == last {
			av := &atomicValue{}
			av.Store(v)
			return av, true
		}
		if m, ok := v.(map[string]any); ok {
			cur = m
		} else {
			return nil, false
		}
	}
	return nil, false
}
