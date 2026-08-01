package config

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	_           Config = (*config)(nil)
	ErrNotFound        = errors.New("config: key not found")
	ErrClosed          = errors.New("config: closed")
)

type Observer func(string, Value)

type Config interface {
	Load() error
	Scan(v any) error
	Value(key string) Value
	Watch(key string, o Observer) error
	Close() error
}

type observerState struct {
	snapshot any
	fn       Observer
}

type config struct {
	lifecycleMu sync.Mutex
	loaded      bool
	closed      bool

	sources   []Source
	reader    *Reader
	opts      options
	cache     sync.Map // string → Value
	observers sync.Map // string → *observerState
	watchers  []Watcher
}

func New(opts ...Option) Config {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &config{
		sources: o.sources,
		reader:  newReader(o),
		opts:    o,
	}
}

func (c *config) Load() error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.closed {
		return ErrClosed
	}
	if c.loaded {
		return nil
	}
	if err := c.reload(); err != nil {
		return err
	}

	watchers := make([]Watcher, 0, len(c.sources))
	for _, src := range c.sources {
		w, err := src.Watch()
		if errors.Is(err, ErrWatchUnsupported) {
			continue
		}
		if err != nil {
			for _, watcher := range watchers {
				watcher.Stop()
			}
			logx.Errorf("config: watch source: %v", err)
			return err
		}
		watchers = append(watchers, w)
	}
	c.watchers = watchers
	c.loaded = true
	for _, watcher := range watchers {
		w := watcher
		help.SafeGo(func() {
			c.watchLoop(w)
		})
	}
	return nil
}

func (c *config) reload() error {
	var kvs []*KeyValue
	for _, src := range c.sources {
		next, err := src.Load()
		if err != nil {
			return err
		}
		kvs = append(kvs, next...)
	}
	if err := c.reader.Load(kvs...); err != nil {
		return err
	}
	c.syncCachedValues()
	return nil
}

func (c *config) Scan(v any) error {
	data, err := c.reader.Source()
	if err != nil {
		return err
	}
	return unmarshalJSON(data, v)
}

func (c *config) Value(key string) Value {
	if v, ok := c.cache.Load(key); ok {
		return v.(Value)
	}
	val, ok := c.reader.Value(key)
	if !ok {
		return nil
	}
	av := &atomicValue{}
	av.Store(clone(val))
	c.cache.Store(key, av)
	return av
}

func (c *config) Watch(key string, o Observer) error {
	val, ok := c.reader.Value(key)
	if !ok || val == nil {
		return ErrNotFound
	}
	c.observers.Store(key, &observerState{
		snapshot: clone(val),
		fn:       o,
	})
	return nil
}

func (c *config) Close() error {
	c.lifecycleMu.Lock()
	if c.closed {
		c.lifecycleMu.Unlock()
		return nil
	}
	c.closed = true
	watchers := c.watchers
	c.watchers = nil
	c.lifecycleMu.Unlock()

	for _, w := range watchers {
		w.Stop()
	}
	return nil
}

func (c *config) watchLoop(w Watcher) {
	for {
		_, err := w.Next()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			logx.Errorf("config: watch next: %v", err)
			continue
		}
		if err := c.reload(); err != nil {
			logx.Errorf("config: reload source: %v", err)
			continue
		}
		c.notifyObservers()
	}
}

func (c *config) syncCachedValues() {
	c.cache.Range(func(key, value any) bool {
		k := key.(string)
		v := value.(Value)
		current, ok := c.reader.Value(k)
		if !ok {
			v.Store(nil)
			return true
		}
		v.Store(clone(current))
		return true
	})
}

func (c *config) notifyObservers() {
	c.observers.Range(func(key, value any) bool {
		k := key.(string)
		s := value.(*observerState)

		current, ok := c.reader.Value(k)
		if !ok {
			current = nil
		}
		if reflect.DeepEqual(s.snapshot, current) {
			return true
		}
		s.snapshot = clone(current)
		nv := &atomicValue{}
		nv.Store(clone(current))
		help.SafeGo(func() {
			s.fn(k, nv)
		})
		return true
	})
}

func Get[T any](c Config, key string) (T, error) {
	var zero T
	v := c.Value(key)
	if v == nil || v.Load() == nil {
		return zero, ErrNotFound
	}
	switch any(zero).(type) {
	case bool:
		b, err := v.Bool()
		return any(b).(T), err
	case int64:
		i, err := v.Int()
		return any(i).(T), err
	case int:
		i, err := v.Int()
		return any(int(i)).(T), err
	case float64:
		f, err := v.Float()
		return any(f).(T), err
	case string:
		s, err := v.String()
		return any(s).(T), err
	}
	if err := v.Scan(&zero); err != nil {
		return zero, err
	}
	return zero, nil
}

func unmarshalJSON(data []byte, v any) error {
	if m, ok := v.(proto.Message); ok {
		return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, m)
	}
	return json.Unmarshal(data, v)
}
