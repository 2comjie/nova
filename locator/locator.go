package locator

import (
	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/registry"
)

const GateName = "gate"

type Provider interface {
	Bind(name string, key string, instanceId string) error
	Unbind(name string, key string) error
	Locate(name string, key string) (string, error)
	Close()
}
type NodeLocator struct {
	provider Provider

	discover registry.Discover
}

func (l *NodeLocator) Bind(name string, key string, instanceId string) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	return l.provider.Bind(name, key, instanceId)
}
func (l *NodeLocator) Unbind(name string, key string) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	return l.provider.Unbind(name, key)
}
func (l *NodeLocator) Locate(name string, key string) (endpoint.ServiceInstance, error) {
	if name == GateName {
		return endpoint.ServiceInstance{}, ErrNodeNotSupport
	}
	instanceId, err := l.provider.Locate(name, key)
	if err != nil {
		return endpoint.ServiceInstance{}, err
	}
	return l.discover.Get(instanceId)
}

type GateLocator struct {
	provider Provider

	discover registry.Discover
}

func NewGateLocator(provider Provider, discover registry.Discover) *GateLocator {
	return &GateLocator{
		provider: provider,
		discover: discover,
	}
}
func NewNodeLocator(provider Provider, discover registry.Discover) *NodeLocator {
	return &NodeLocator{
		provider: provider,
		discover: discover,
	}
}
func (l *GateLocator) Bind(key string, instanceId string) error {
	return l.provider.Bind(GateName, key, instanceId)
}
func (l *GateLocator) Unbind(key string) error {
	return l.provider.Unbind(GateName, key)
}
func (l *GateLocator) Locate(key string) (endpoint.ServiceInstance, error) {
	instanceId, err := l.provider.Locate(GateName, key)
	if err != nil {
		return endpoint.ServiceInstance{}, err
	}
	return l.discover.Get(instanceId)
}
