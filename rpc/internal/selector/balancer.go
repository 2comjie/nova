package selector

import (
	"context"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/registry"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
)

type Balancer struct {
	cc       balancer.ClientConn
	picker   *Picker
	discover registry.Discover
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewBalancer(dis registry.Discover, loc locator.Locator, cc balancer.ClientConn) *Balancer {
	ctx, cancel := context.WithCancel(context.Background())
	picker := newPicker(cc, loc, dis)
	b := &Balancer{
		cc:       cc,
		picker:   picker,
		discover: dis,
		ctx:      ctx,
		cancel:   cancel,
	}
	instances, err := b.discover.List(b.ctx)
	if err == nil {
		b.picker.updateIndex(instances)
		// 预建 SubConn，避免首次调用时才建连接导致超时
		for _, inst := range instances {
			picker.getOrCreateSubConn(inst.RpcTarget())
		}
	}
	b.cc.UpdateState(balancer.State{
		ConnectivityState: connectivity.Connecting,
		Picker:            picker,
	})
	help.SafeGo(b.watch)
	return b
}

func (b *Balancer) UpdateClientConnState(state balancer.ClientConnState) error {
	// 解析器地址变更，不需要处理，Picker 自己管理地址
	return nil
}

func (b *Balancer) ResolverError(err error) {
	logx.Error("selector resolver error", err)
}

func (b *Balancer) UpdateSubConnState(conn balancer.SubConn, state balancer.SubConnState) {
	switch state.ConnectivityState {
	case connectivity.Ready:
		b.cc.UpdateState(balancer.State{
			ConnectivityState: connectivity.Ready,
			Picker:            b.picker,
		})
	case connectivity.TransientFailure:
		b.cc.UpdateState(balancer.State{
			ConnectivityState: connectivity.TransientFailure,
			Picker:            b.picker,
		})
	}
}

func (b *Balancer) Close() {
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *Balancer) ExitIdle() {}

func (b *Balancer) watch() {
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			instances, err := b.discover.Next(b.ctx)
			if err != nil {
				logx.Error("selector watch error", err)
				continue
			}
			b.picker.updateIndex(instances)
		}
	}
}
