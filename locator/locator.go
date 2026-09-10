package locator

import (
	"context"
	"encoding/json"

	"github.com/spf13/cast"
)

const GateName = "gate"

type Locator interface {
	SetOnBindingLost(callback func(name, key, value string))
	Bind(ctx context.Context, name string, key string, value string) (previous string, err error)
	Unbind(ctx context.Context, name string, key string, instanceId string) error
	Locate(ctx context.Context, name string, key string) (string, error)
	Close()
}

type NodeLocator struct {
	provider Locator
}

func (l *NodeLocator) Bind(ctx context.Context, name string, key string, instanceId string) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	_, err := l.provider.Bind(ctx, name, key, instanceId)
	return err
}

func (l *NodeLocator) Unbind(ctx context.Context, name string, key string, instanceId string) error {
	if name == GateName {
		return ErrNodeNotSupport
	}
	return l.provider.Unbind(ctx, name, key, instanceId)
}

func (l *NodeLocator) Locate(ctx context.Context, name string, key string) (string, error) {
	if name == GateName {
		return "", ErrNodeNotSupport
	}
	return l.provider.Locate(ctx, name, key)
}

func (l *NodeLocator) Close() {
	l.provider.Close()
}

type GateLocator struct {
	provider Locator
}

type GateBinding struct {
	InstanceId string `json:"instance_id"`
	SessionId  uint64 `json:"session_id"`
}

func NewGateLocator(provider Locator) *GateLocator {
	return &GateLocator{provider: provider}
}

func NewNodeLocator(provider Locator) *NodeLocator {
	return &NodeLocator{provider: provider}
}

func (l *GateLocator) SetOnBindingLost(callback func(uid uint64, binding GateBinding)) {
	l.provider.SetOnBindingLost(func(name, key, value string) {
		if name != GateName {
			return
		}
		uid, err := cast.ToUint64E(value)
		if err != nil {
			panic(err)
		}
		var binding GateBinding
		if err := json.Unmarshal([]byte(value), &binding); err != nil {
			panic(err)
		}
		callback(uid, binding)
	})
}

func (l *GateLocator) Bind(ctx context.Context, uid uint64, binding GateBinding) error {
	bindBs, _ := json.Marshal(binding)
	_, err := l.provider.Bind(ctx, GateName, cast.ToString(uid), string(bindBs))
	return err
}

func (l *GateLocator) Unbind(ctx context.Context, uid uint64, binding GateBinding) error {
	bindBs, _ := json.Marshal(binding)
	return l.provider.Unbind(ctx, GateName, cast.ToString(uid), string(bindBs))
}

func (l *GateLocator) Locate(ctx context.Context, uid uint64) (string, error) {
	binding, err := l.LocateBinding(ctx, uid)
	return binding.InstanceId, err
}

func (l *GateLocator) LocateBinding(ctx context.Context, uid uint64) (GateBinding, error) {
	value, err := l.provider.Locate(ctx, GateName, cast.ToString(uid))
	if err != nil || value == "" {
		return GateBinding{}, err
	}
	var binding GateBinding
	if err := json.Unmarshal([]byte(value), &binding); err != nil {
		return GateBinding{}, err
	}
	return binding, nil
}

func (l *GateLocator) Close() {
	l.provider.Close()
}
