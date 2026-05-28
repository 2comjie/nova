package config

type Source interface {
	Load() ([]*KeyValue, error)
	Watch() (Watcher, error)
}

type Watcher interface {
	Next() ([]*KeyValue, error)
	Stop()
}
