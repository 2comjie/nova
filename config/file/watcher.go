package file

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/wali/config"
	"github.com/fsnotify/fsnotify"
)

type watcher struct {
	fsWatcher *fsnotify.Watcher
	root      string
	target    string
	recursive bool

	ctx    context.Context
	cancel context.CancelFunc
	stop   sync.Once
}

func newWatcher(sourcePath string) (config.Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		fsWatcher.Close()
		return nil, err
	}

	cleanPath := filepath.Clean(sourcePath)
	w := &watcher{fsWatcher: fsWatcher}
	if info.IsDir() {
		w.root = cleanPath
		w.recursive = true
		if err := w.addDirectories(cleanPath); err != nil {
			fsWatcher.Close()
			return nil, err
		}
	} else {
		w.root = filepath.Dir(cleanPath)
		w.target = cleanPath
		if err := fsWatcher.Add(w.root); err != nil {
			fsWatcher.Close()
			return nil, err
		}
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	return w, nil
}

func (w *watcher) Next() error {
	for {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case event, open := <-w.fsWatcher.Events:
			if !open {
				return context.Canceled
			}
			if !w.relevant(event) {
				continue
			}
			w.watchCreatedDirectory(event)
			return w.debounce()
		case err, open := <-w.fsWatcher.Errors:
			if !open {
				return context.Canceled
			}
			return err
		}
	}
}

func (w *watcher) debounce() error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case event, open := <-w.fsWatcher.Events:
			if !open {
				return context.Canceled
			}
			if !w.relevant(event) {
				continue
			}
			w.watchCreatedDirectory(event)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(100 * time.Millisecond)
		case err, open := <-w.fsWatcher.Errors:
			if !open {
				return context.Canceled
			}
			return err
		case <-timer.C:
			return nil
		}
	}
}

func (w *watcher) relevant(event fsnotify.Event) bool {
	if event.Op&^fsnotify.Chmod == 0 {
		return false
	}
	cleanEventPath := filepath.Clean(event.Name)
	if !w.recursive {
		return cleanEventPath == w.target
	}
	relativePath, err := filepath.Rel(w.root, cleanEventPath)
	return err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(os.PathSeparator))
}

func (w *watcher) watchCreatedDirectory(event fsnotify.Event) {
	if !w.recursive || !event.Has(fsnotify.Create) {
		return
	}
	info, err := os.Stat(event.Name)
	if err == nil && info.IsDir() {
		_ = w.addDirectories(event.Name)
	}
}

func (w *watcher) addDirectories(root string) error {
	return filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if currentPath != root && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		return w.fsWatcher.Add(currentPath)
	})
}

func (w *watcher) Stop() {
	w.stop.Do(func() {
		w.cancel()
		_ = w.fsWatcher.Close()
	})
}
