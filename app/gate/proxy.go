package gate

import (
	"context"
	"maps"

	"github.com/2comjie/wali/core/endpoint"
)

type Proxy struct {
	app *Gate
}

func (p *Proxy) Push(ctx context.Context, uid string, route uint32, body []byte) error {
	if p == nil || p.app == nil || p.app.server == nil {
		return ErrClosed
	}
	return p.app.server.PushUID(ctx, uid, route, body)
}

func (p *Proxy) KickUID(uid string) bool {
	return p != nil && p.app != nil && p.app.server != nil && p.app.server.KickUID(uid)
}

func (p *Proxy) KickSession(sessionID uint64) bool {
	return p != nil && p.app != nil && p.app.server != nil && p.app.server.KickSession(sessionID)
}

func (p *Proxy) Instance() endpoint.ServiceInstance {
	if p == nil || p.app == nil {
		return endpoint.ServiceInstance{}
	}
	instance := p.app.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}
