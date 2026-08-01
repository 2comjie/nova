package gate

import (
	"context"
	"maps"

	"github.com/2comjie/wali/core/endpoint"
)

type Proxy struct {
	app *Gate
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

func (p *Proxy) Push(ctx context.Context, uid string, route uint32, body []byte) error {
	return p.app.server.PushUID(ctx, uid, route, body)
}

func (p *Proxy) KickUID(uid string) bool {
	return p.app.server.KickUID(uid)
}

func (p *Proxy) KickSession(sessionID uint64) bool {
	return p.app.server.KickSession(sessionID)
}

func (p *Proxy) Instance() endpoint.ServiceInstance {
	instance := p.app.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}
