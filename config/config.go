package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	ErrNotFound = errors.New("config: key not found")
	ErrClosed   = errors.New("config: closed")
)

type Observer func(string, Value)

type Config interface {
	Load() error
	Scan(any) error
	Value(string) Value
	Watch(string, Observer) error
	Close() error
}

type snapshot struct {
	root map[string]any
}

type observerState struct {
	snapshot any
	observer Observer
}

type config struct {
	sources []Source
	current atomic.Pointer[snapshot]

	lifecycleMu sync.Mutex
	loaded      bool
	closed      bool
	watchers    []Watcher

	reloadMu sync.Mutex

	valuesMu sync.Mutex
	values   map[string]*atomicValue

	observersMu sync.Mutex
	observers   map[string]*observerState
}

func New(opts ...Option) Config {
	var settings options
	for _, option := range opts {
		option(&settings)
	}
	center := &config{
		sources:   append([]Source(nil), settings.sources...),
		values:    make(map[string]*atomicValue),
		observers: make(map[string]*observerState),
	}
	center.current.Store(&snapshot{root: make(map[string]any)})
	return center
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
	if err := c.reload(false); err != nil {
		return err
	}

	watchers := make([]Watcher, 0, len(c.sources))
	for index, source := range c.sources {
		watcher, err := source.Watch()
		if errors.Is(err, ErrWatchUnsupported) {
			continue
		}
		if err != nil {
			for _, created := range watchers {
				created.Stop()
			}
			return fmt.Errorf("config: watch source %d: %w", index, err)
		}
		if watcher == nil {
			for _, created := range watchers {
				created.Stop()
			}
			return fmt.Errorf("config: watch source %d returned nil watcher", index)
		}
		watchers = append(watchers, watcher)
	}

	c.watchers = watchers
	c.loaded = true
	for _, watcher := range watchers {
		current := watcher
		help.SafeGo(func() {
			c.watchLoop(current)
		})
	}
	return nil
}

func (c *config) Scan(target any) error {
	data, err := json.Marshal(c.current.Load().root)
	if err != nil {
		return err
	}
	return unmarshalJSON(data, target)
}

func (c *config) Value(key string) Value {
	c.valuesMu.Lock()
	defer c.valuesMu.Unlock()
	if value, exists := c.values[key]; exists {
		return value
	}
	current, exists := readTree(c.current.Load().root, key)
	if !exists {
		return nil
	}
	value := &atomicValue{}
	value.Store(clone(current))
	c.values[key] = value
	return value
}

func (c *config) Watch(key string, observer Observer) error {
	if observer == nil {
		return errors.New("config: observer is nil")
	}
	current, exists := readTree(c.current.Load().root, key)
	if !exists {
		return ErrNotFound
	}
	c.observersMu.Lock()
	c.observers[key] = &observerState{
		snapshot: clone(current),
		observer: observer,
	}
	c.observersMu.Unlock()
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

	for _, watcher := range watchers {
		watcher.Stop()
	}
	return nil
}

func (c *config) reload(notify bool) error {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()
	root, err := buildTree(c.sources)
	if err != nil {
		return err
	}
	c.current.Store(&snapshot{root: root})
	c.syncValues(root)
	if notify {
		c.notifyObservers(root)
	}
	return nil
}

func (c *config) watchLoop(watcher Watcher) {
	for {
		if err := watcher.Next(); err != nil {
			if errors.Is(err, context.Canceled) || c.isClosed() {
				return
			}
			logx.Errorf("config: watch source: %v", err)
			continue
		}
		if c.isClosed() {
			return
		}
		if err := c.reload(true); err != nil {
			logx.Errorf("config: reload source: %v", err)
		}
	}
}

func (c *config) isClosed() bool {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	return c.closed
}

func (c *config) syncValues(root map[string]any) {
	c.valuesMu.Lock()
	defer c.valuesMu.Unlock()
	for key, value := range c.values {
		current, exists := readTree(root, key)
		if !exists {
			value.Store(nil)
			continue
		}
		value.Store(clone(current))
	}
}

func (c *config) notifyObservers(root map[string]any) {
	type notification struct {
		key      string
		value    Value
		observer Observer
	}

	c.observersMu.Lock()
	notifications := make([]notification, 0, len(c.observers))
	for key, state := range c.observers {
		current, exists := readTree(root, key)
		if !exists {
			current = nil
		}
		if reflect.DeepEqual(state.snapshot, current) {
			continue
		}
		state.snapshot = clone(current)
		value := &atomicValue{}
		value.Store(clone(current))
		notifications = append(notifications, notification{
			key:      key,
			value:    value,
			observer: state.observer,
		})
	}
	c.observersMu.Unlock()

	for _, event := range notifications {
		current := event
		help.SafeGo(func() {
			current.observer(current.key, current.value)
		})
	}
}

func Get[T any](center Config, key string) (T, error) {
	var zero T
	value := center.Value(key)
	if value == nil || value.Load() == nil {
		return zero, ErrNotFound
	}
	switch any(zero).(type) {
	case bool:
		result, err := value.Bool()
		return any(result).(T), err
	case int:
		result, err := value.Int()
		return any(int(result)).(T), err
	case int64:
		result, err := value.Int()
		return any(result).(T), err
	case float64:
		result, err := value.Float()
		return any(result).(T), err
	case string:
		result, err := value.String()
		return any(result).(T), err
	}
	if err := value.Scan(&zero); err != nil {
		return zero, err
	}
	return zero, nil
}

func unmarshalJSON(data []byte, target any) error {
	if message, ok := target.(proto.Message); ok {
		return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, message)
	}
	return json.Unmarshal(data, target)
}
