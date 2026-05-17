package registry

import "github.com/2comjie/wali/core/endpoint"

type Registry interface {
	Register(instance endpoint.ServiceInstance) error
	Deregister(instanceID string) error
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
