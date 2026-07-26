package gate

import (
	"context"
	"maps"

	"github.com/2comjie/wali/core/endpoint"
)

type Proxy struct {
	app *Gate
}

// AddWait 注册一个需要在Gate关闭时等待的后台任务。
func (p *Proxy) AddWait() {
	p.app.AddWait()
}

// DoneWait 标记一个后台任务已经退出。
func (p *Proxy) DoneWait() {
	p.app.DoneWait()
}

// Wait 等待Gate管理的全部后台任务退出。
func (p *Proxy) Wait() {
	p.app.Wait()
}

// Done 在Gate停止后台任务时关闭。
func (p *Proxy) Done() <-chan struct{} {
	return p.app.Done()
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
