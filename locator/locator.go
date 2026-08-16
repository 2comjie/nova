package locator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/2comjie/nova/registry"
)

const GateName = "gate"

type Locator interface {
	Bind(ctx context.Context, name string, key string, value string) (previous string, err error)
	Restore(ctx context.Context, name string, key string, current string, previous string) (bool, error)
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
	instanceId, err := l.provider.Locate(ctx, name, key)
	if err != nil {
		return "", err
	}
	return instanceId, nil
}

func (l *NodeLocator) Close() {
	l.provider.Close()
}

type GateLocator struct {
	provider Locator

	discover registry.Discover
}

type GateBinding struct {
	InstanceID string `json:"instance_id"`
	SessionID  uint64 `json:"session_id"`
}

func NewGateLocator(provider Locator, discover registry.Discover) *GateLocator {
	return &GateLocator{
		provider: provider,
		discover: discover,
	}
}

func NewNodeLocator(provider Locator) *NodeLocator {
	return &NodeLocator{provider: provider}
}

func (l *GateLocator) Bind(ctx context.Context, uid string, binding GateBinding) (GateBinding, error) {
	value, err := encodeGateBinding(binding)
	if err != nil {
		return GateBinding{}, err
	}
	previous, err := l.provider.Bind(ctx, GateName, uid, value)
	if err != nil || previous == "" {
		return GateBinding{}, err
	}
	return decodeGateBinding(previous)
}

func (l *GateLocator) Restore(ctx context.Context, uid string, current GateBinding, previous GateBinding) (bool, error) {
	currentValue, err := encodeGateBinding(current)
	if err != nil {
		return false, err
	}
	previousValue, err := encodeGateBinding(previous)
	if err != nil {
		return false, err
	}
	return l.provider.Restore(ctx, GateName, uid, currentValue, previousValue)
}

func (l *GateLocator) Unbind(ctx context.Context, uid string, binding GateBinding) error {
	value, err := encodeGateBinding(binding)
	if err != nil {
		return err
	}
	return l.provider.Unbind(ctx, GateName, uid, value)
}

func (l *GateLocator) Locate(ctx context.Context, uid string) (string, error) {
	value, err := l.provider.Locate(ctx, GateName, uid)
	if err != nil || value == "" {
		return "", err
	}
	binding, err := decodeGateBinding(value)
	if err != nil {
		return "", err
	}
	return binding.InstanceID, nil
}

func (l *GateLocator) Close() {
	l.provider.Close()
}

func encodeGateBinding(binding GateBinding) (string, error) {
	if binding.InstanceID == "" || binding.SessionID == 0 {
		return "", fmt.Errorf("locator: GateBinding无效 instance=%q session=%d", binding.InstanceID, binding.SessionID)
	}
	value, err := json.Marshal(binding)
	if err != nil {
		return "", fmt.Errorf("locator: 编码GateBinding失败: %w", err)
	}
	return string(value), nil
}

func decodeGateBinding(value string) (GateBinding, error) {
	var binding GateBinding
	if err := json.Unmarshal([]byte(value), &binding); err != nil {
		return GateBinding{}, fmt.Errorf("locator: 解析GateBinding失败: %w", err)
	}
	if binding.InstanceID == "" || binding.SessionID == 0 {
		return GateBinding{}, fmt.Errorf("locator: GateBinding无效 instance=%q session=%d", binding.InstanceID, binding.SessionID)
	}
	return binding, nil
}
