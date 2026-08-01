package config

import "errors"

var ErrWatchUnsupported = errors.New("config: watch unsupported")

type Source interface {
	Load() ([]*KeyValue, error)
	Watch() (Watcher, error)
}

type Watcher interface {
	Next() ([]*KeyValue, error)
	Stop()
}
