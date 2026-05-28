package config

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"

	"github.com/2comjie/wali/core/help"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	_           Config = (*config)(nil)
	ErrNotFound        = errors.New("config: key not found")
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
	for _, src := range c.sources {
		kvs, err := src.Load()
		if err != nil {
			return err
		}
		if err := c.reader.Merge(kvs...); err != nil {
			zap.S().Errorf("config: merge source: %v", err)
			return err
		}
		w, err := src.Watch()
		if err != nil {
			zap.S().Errorf("config: watch source: %v", err)
			return err
		}
		c.watchers = append(c.watchers, w)
		help.SafeGo(func() {
			c.watchLoop(w)
		})
	}
	return c.reader.Resolve()
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
	av, ok := readValue(c.reader.values, key)
	if !ok {
		return nil
	}
	c.cache.Store(key, av)
	return av
}

func (c *config) Watch(key string, o Observer) error {
	v, _ := readValue(c.reader.values, key)
	if v == nil || v.Load() == nil {
		return ErrNotFound
	}
	c.observers.Store(key, &observerState{
		snapshot: clone(v.Load()),
		fn:       o,
	})
	return nil
}

func (c *config) Close() error {
	for _, w := range c.watchers {
		w.Stop()
	}
	return nil
}

func (c *config) watchLoop(w Watcher) {
	for {
		kvs, err := w.Next()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			zap.S().Errorf("config: watch next: %v", err)
			continue
		}
		if err := c.reader.Merge(kvs...); err != nil {
			zap.S().Errorf("config: watch merge: %v", err)
			continue
		}
		if err := c.reader.Resolve(); err != nil {
			zap.S().Errorf("config: watch resolve: %v", err)
			continue
		}
		c.notifyObservers()
	}
}

func (c *config) notifyObservers() {
	c.observers.Range(func(key, value any) bool {
		k := key.(string)
		s := value.(*observerState)

		nv, ok := readValue(c.reader.values, k)
		if !ok {
			return true
		}
		current := nv.Load()
		if reflect.DeepEqual(s.snapshot, current) {
			return true
		}
		s.snapshot = clone(current)
		if cv, ok := c.cache.Load(k); ok {
			cv.(Value).Store(current)
		}
		s.fn(k, nv)
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
