package config

import (
	"sync/atomic"

	"github.com/2comjie/nova/logx"
)

// WatchedValue 保存指定配置键的类型化只读快照，并在配置变化时原子替换。
type WatchedValue[T any] struct {
	current atomic.Pointer[T]
}

// Init 加载指定配置键并监听后续变化。
// 热更新解析失败时保留上一份有效配置。
func (w *WatchedValue[T]) Init(center Config, key string) error {
	value := center.Value(key)
	if value == nil {
		return ErrNotFound
	}
	if err := w.store(value); err != nil {
		return err
	}

	if err := center.Watch(key, func(_ string, _ Value) {
		// Value由配置中心在通知前原子更新。始终读取它的最新值，
		// 避免多个异步通知发生调度乱序时旧值覆盖新值。
		if err := w.store(value); err != nil {
			logx.Errorf("config: reload watched value key=%s: %v", key, err)
		}
	}); err != nil {
		return err
	}

	// 关闭首次加载与注册监听之间可能错过更新的窗口。
	return w.store(value)
}

func (w *WatchedValue[T]) store(value Value) error {
	if value.Load() == nil {
		return ErrNotFound
	}
	var current T
	if err := value.Scan(&current); err != nil {
		return err
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
