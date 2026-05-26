package registry

import (
	"context"

	"github.com/2comjie/wali/core/endpoint"
)

type Registry interface {
	Register(instance endpoint.ServiceInstance) error
	Deregister(instanceID string) error
	Close()
}
type Discover interface {
	List(ctx context.Context) (map[string]endpoint.ServiceInstance, error)
	Next(ctx context.Context) (map[string]endpoint.ServiceInstance, error)
	Get(ctx context.Context, nstanceID string) (endpoint.ServiceInstance, error)
	Close()
}
