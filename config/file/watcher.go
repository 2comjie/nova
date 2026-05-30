package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/2comjie/wali/config"
	"github.com/fsnotify/fsnotify"
)

var _ config.Watcher = (*watcher)(nil)

type watcher struct {
	f         *file
	fw        *fsnotify.Watcher
	watchPath string
	isDir     bool

	ctx    context.Context
	cancel context.CancelFunc
}

func newWatcher(f *file) (config.Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(f.path)
	if err != nil {
		fw.Close()
		return nil, err
	}
	watchPath := f.path
	if !fi.IsDir() {
		watchPath = filepath.Dir(f.path)
	}
	if err := fw.Add(watchPath); err != nil {
		fw.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &watcher{f: f, fw: fw, watchPath: watchPath, isDir: fi.IsDir(), ctx: ctx, cancel: cancel}, nil
}

func (w *watcher) Next() ([]*config.KeyValue, error) {
	for {
		select {
		case <-w.ctx.Done():
			return nil, w.ctx.Err()
		case event, ok := <-w.fw.Events:
			if !ok {
				return nil, context.Canceled
			}
			if !w.relevant(event) {
				continue
			}
			w.debounce()
			return w.f.Load()
		case err, ok := <-w.fw.Errors:
			if !ok {
				return nil, context.Canceled
			}
			return nil, err
		}
	}
}

func (w *watcher) relevant(event fsnotify.Event) bool {
	if event.Has(fsnotify.Chmod) {
		return false
	}
	if w.isDir {
		return strings.HasPrefix(filepath.Clean(event.Name), filepath.Clean(w.f.path)+string(os.PathSeparator))
	}
	return filepath.Clean(event.Name) == filepath.Clean(w.f.path)
}

func (w *watcher) debounce() {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case event := <-w.fw.Events:
			if w.relevant(event) {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(100 * time.Millisecond)
			}
		case <-timer.C:
			return
		}
	}
}

func (w *watcher) Stop() {
	w.cancel()
	w.fw.Close()
}
