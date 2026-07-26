package node

import (
	"context"
	"errors"
	"maps"

	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	"github.com/2comjie/wali/rpc/lx"
)

var (
	ErrInvalidUID     = errors.New("node: UID不能为空")
	ErrInvalidKey     = errors.New("node: Locator key不能为空")
	ErrInvalidBinding = errors.New("node: Locator binding不能为空")
)

// Proxy 是 Node 暴露给 Handler 和 Middleware 的能力门面。
// Proxy 不暴露 Registry、Locator 和 RPC Client 等内部组件。
type Proxy struct {
	app *Node
}

// Push 向指定UID所在的Gate推送消息。
func (p *Proxy) Push(ctx context.Context, uid string, route uint32, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uid == "" {
		return ErrInvalidUID
	}
	if route == 0 {
		return ErrInvalidRoute
	}

	gateInstanceID, err := p.app.gateLocator.Locate(ctx, uid)
	if err != nil {
		return err
	}
	_, err = p.app.gateClient.Push(lx.WithNode(ctx, gateInstanceID), &pbGate.PushRequest{
		Uid:             uid,
		Route:           route,
		Body:            body,
		NodeServiceName: p.app.instance.ServiceName,
		NodeInstanceId:  p.app.instance.ID,
	})
	return err
}

// Kick 关闭指定UID所在Gate上的客户端连接。
func (p *Proxy) Kick(ctx context.Context, uid string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uid == "" {
		return ErrInvalidUID
	}

	gateInstanceID, err := p.app.gateLocator.Locate(ctx, uid)
	if err != nil {
		return err
	}
	_, err = p.app.gateClient.Kick(lx.WithNode(ctx, gateInstanceID), &pbGate.KickRequest{
		Uid:             uid,
		NodeServiceName: p.app.instance.ServiceName,
		NodeInstanceId:  p.app.instance.ID,
	})
	return err
}

// Bind 将指定类型的游戏状态绑定到当前Node实例。
func (p *Proxy) Bind(ctx context.Context, binding string, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if binding == "" {
		return ErrInvalidBinding
	}
	if key == "" {
		return ErrInvalidKey
	}
	return p.app.nodeLocator.Bind(ctx, binding, key, p.app.instance.ID)
}

// Unbind 解除指定类型游戏状态与当前Node实例的绑定。
func (p *Proxy) Unbind(ctx context.Context, binding string, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if binding == "" {
		return ErrInvalidBinding
	}
	if key == "" {
		return ErrInvalidKey
	}
	return p.app.nodeLocator.Unbind(ctx, binding, key, p.app.instance.ID)
}

// Locate 查找指定类型游戏状态当前绑定的Node实例。
func (p *Proxy) Locate(ctx context.Context, binding string, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if binding == "" {
		return "", ErrInvalidBinding
	}
	if key == "" {
		return "", ErrInvalidKey
	}
	return p.app.nodeLocator.Locate(ctx, binding, key)
}

// Instance 返回当前Node实例信息的副本。
func (p *Proxy) Instance() endpoint.ServiceInstance {
	instance := p.app.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}
