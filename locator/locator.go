package locator

import (
	"context"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/registry"
)

const GateName = "gate"

type Locator interface {
	Bind(ctx context.Context, name string, key string, instanceId string) error
	Unbind(ctx context.Context, name string, key string) error
	Locate(ctx context.Context, name string, key string) (string, error)
	Close()
}
type NodeLocator struct {
	provider Locator

	discover registry.Discover
}

func (l *NodeLocator) Bind(ctx context.Context, name string, key string, instanceId string) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	return l.provider.Bind(ctx, name, key, instanceId)
}
func (l *NodeLocator) Unbind(ctx context.Context, name string, key string) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	return l.provider.Unbind(ctx, name, key)
}
func (l *NodeLocator) Locate(ctx context.Context, name string, key string) (endpoint.ServiceInstance, error) {
	if name == GateName {
		return endpoint.ServiceInstance{}, ErrNodeNotSupport
	}
	instanceId, err := l.provider.Locate(ctx, name, key)
	if err != nil {
		return endpoint.ServiceInstance{}, err
	}
	return l.discover.Get(ctx, instanceId)
}

type GateLocator struct {
	provider Locator

	discover registry.Discover
}

func NewGateLocator(provider Locator, discover registry.Discover) *GateLocator {
	return &GateLocator{
		provider: provider,
		discover: discover,
	}
}
func NewNodeLocator(provider Locator, discover registry.Discover) *NodeLocator {
	return &NodeLocator{
		provider: provider,
		discover: discover,
	}
}
func (l *GateLocator) Bind(ctx context.Context, key string, instanceId string) error {
	return l.provider.Bind(ctx, GateName, key, instanceId)
}
func (l *GateLocator) Unbind(ctx context.Context, key string) error {
	return l.provider.Unbind(ctx, GateName, key)
}
func (l *GateLocator) Locate(ctx context.Context, key string) (endpoint.ServiceInstance, error) {
	instanceId, err := l.provider.Locate(ctx, GateName, key)
	if err != nil {
		return endpoint.ServiceInstance{}, err
	}
	return l.discover.Get(ctx, instanceId)
}
