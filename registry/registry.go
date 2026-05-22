package registry

import "github.com/2comjie/wali/core/endpoint"

type Registry interface {
	Register(instance endpoint.ServiceInstance) error
	Deregister(instanceID string) error
	Close()
}
type Discover interface {
	List() (map[string]endpoint.ServiceInstance, error)
	Next() (map[string]endpoint.ServiceInstance, error)
	Get(instanceID string) (endpoint.ServiceInstance, error)
	Close()
}
