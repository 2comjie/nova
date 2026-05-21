package locator

import (
	"github.com/2comjie/wali/core/endpoint"
)

const GateName = "gate"

type Provider interface {
	Bind(name string, key string, endpoint endpoint.ServiceInstance) error
	Unbind(name string, key string) error
	Locate(name string, key string) (endpoint.ServiceInstance, error)
}

type NodeLocator struct {
	provider Provider
}

func (l *NodeLocator) Bind(name string, key string, instance endpoint.ServiceInstance) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	return l.provider.Bind(name, key, instance)
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
	return l.provider.Locate(name, key)
}

type GateLocator struct {
	provider Provider
}

func (l *GateLocator) Bind(key string, instance endpoint.ServiceInstance) error {
	return l.provider.Bind(GateName, key, instance)
}
func (l *GateLocator) Unbind(key string) error {
	return l.provider.Unbind(GateName, key)
}
func (l *GateLocator) Locate(key string) (endpoint.ServiceInstance, error) {
	return l.provider.Locate(GateName, key)
}
