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

type Proxy struct {
	app *Node
}

func (p *Proxy) AddWait() {
	p.app.AddWait()
}

func (p *Proxy) DoneWait() {
	p.app.DoneWait()
}

func (p *Proxy) Wait() {
	p.app.Wait()
}

func (p *Proxy) Done() <-chan struct{} {
	return p.app.Done()
}
func (p *Proxy) UpdateMetadata(metadata map[string]string) error {
	return p.app.UpdateMetadata(metadata)
}
func (p *Proxy) DeleteMetadata(keys ...string) error {
	return p.app.DeleteMetadata(keys...)
}
func (p *Proxy) RandString() string {
	return p.app.RandString()
}

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

func (p *Proxy) Instance() endpoint.ServiceInstance {
	instance := p.app.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}

func (n *Proxy) Broadcast(ctx context.Context, route uint32, body []byte) (uint32, error) {
	return n.app.Broadcast(ctx, route, body)
}
func (n *Proxy) MultiPush(ctx context.Context, uidList []string, route uint32, body []byte) (uint32, error) {
	return n.app.MultiPush(ctx, uidList, route, body)
}
