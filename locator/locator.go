package locator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
		uid, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			panic(err)
		}
		binding, err := decodeGateBinding(value)
		if err != nil {
			panic(err)
		}
		callback(uid, binding)
	})
}

func (l *GateLocator) Bind(ctx context.Context, uid uint64, binding GateBinding) error {
	_, err := l.provider.Bind(ctx, GateName, strconv.FormatUint(uid, 10), encodeGateBinding(binding))
	return err
}

func (l *GateLocator) Unbind(ctx context.Context, uid uint64, binding GateBinding) error {
	return l.provider.Unbind(ctx, GateName, strconv.FormatUint(uid, 10), encodeGateBinding(binding))
}

func (l *GateLocator) Locate(ctx context.Context, uid uint64) (string, error) {
	binding, err := l.LocateBinding(ctx, uid)
	return binding.InstanceId, err
}

func (l *GateLocator) LocateBinding(ctx context.Context, uid uint64) (GateBinding, error) {
	value, err := l.provider.Locate(ctx, GateName, strconv.FormatUint(uid, 10))
	if err != nil || value == "" {
		return GateBinding{}, err
	}
	return decodeGateBinding(value)
}

func (l *GateLocator) Close() {
	l.provider.Close()
}

func encodeGateBinding(binding GateBinding) string {
	if binding.InstanceId == "" || binding.SessionId == 0 {
		panic("locator: GateBinding缺少InstanceId或SessionId")
	}
	value, _ := json.Marshal(binding)
	return string(value)
}

func decodeGateBinding(value string) (GateBinding, error) {
	var binding GateBinding
	if err := json.Unmarshal([]byte(value), &binding); err != nil {
		return GateBinding{}, err
	}
	if binding.InstanceId == "" || binding.SessionId == 0 {
		return GateBinding{}, fmt.Errorf("locator: GateBinding无效 instance=%q session=%d", binding.InstanceId, binding.SessionId)
	}
	return binding, nil
}
