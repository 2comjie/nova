package config

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/logx"
)

// WatchedValue 保存指定配置键的类型化只读快照，并在配置变化时原子替换。
type WatchedValue[T any] struct {
	mu          sync.Mutex
	initialized bool
	key         string
	current     atomic.Pointer[T]
}

// Init 加载指定配置键并监听后续变化。
// 热更新解析失败时保留上一份有效配置。
func (w *WatchedValue[T]) Init(center Config, key string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.initialized {
		return fmt.Errorf("config: watched value already initialized: %s", w.key)
	}

	value := center.Value(key)
	if value == nil || value.Load() == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	if err := w.store(key, value); err != nil {
		return err
	}

	if err := center.Watch(key, func(_ string, _ Value) {
		// Value由配置中心在通知前原子更新。始终读取它的最新值，
		// 避免多个异步通知发生调度乱序时旧值覆盖新值。
		if err := w.store(key, value); err != nil {
			logx.Errorf("config: reload watched value key=%s: %v", key, err)
		}
	}); err != nil {
		return fmt.Errorf("config: watch key %s: %w", key, err)
	}

	// 关闭首次加载与注册监听之间可能错过更新的窗口。
	if err := w.store(key, value); err != nil {
		logx.Errorf("config: refresh watched value key=%s: %v", key, err)
	}
	w.key = key
	w.initialized = true
	return nil
}

func (w *WatchedValue[T]) store(key string, value Value) error {
	if value.Load() == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	var current T
	if err := value.Scan(&current); err != nil {
		return fmt.Errorf("config: scan key %s: %w", key, err)
	}
	w.current.Store(&current)
	return nil
}

// Load 返回当前配置快照。Map、Slice和指针应当视为只读。
func (w *WatchedValue[T]) Load() T {
	current := w.current.Load()
	if current == nil {
		var zero T
		return zero
	}
	return *current
}
