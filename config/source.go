package config

import "errors"

var ErrWatchUnsupported = errors.New("config: watch unsupported")

type Document struct {
	Path   string
	Format string
	Data   []byte
}

type Source interface {
	Load() ([]Document, error)
	Watch() (Watcher, error)
}

type Watcher interface {
	Next() error
	Stop()
}
