package registry

import "github.com/2comjie/wali/core/endpoint"

type Register interface {
	Registry(service endpoint.ServiceInstance) error
	Close()
}

type Discover interface {
	Watch() (Watcher, error)
	List() ([]*endpoint.ServiceInstance, error)
}

type Watcher interface {
	Next() ([]*endpoint.ServiceInstance, error)
	Close()
}
