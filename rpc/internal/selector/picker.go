package selector

import (
	"context"
	"sync"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/registry"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

type wrrNode struct {
	instance      endpoint.ServiceInstance
	currentWeight int
}

// serviceNodes 持有单个服务的 WRR 状态，锁粒度下沉到 service 级别。
type serviceNodes struct {
	mu    sync.Mutex
	nodes []*wrrNode
}

type Picker struct {
	cc       balancer.ClientConn
	locator  locator.Locator
	discover registry.Discover

	mu       sync.RWMutex
	subConns map[string]balancer.SubConn    // addr → subConn
	svcIndex map[string]*serviceNodes       // serviceName → 节点列表
}

func newPicker(cc balancer.ClientConn, loc locator.Locator, dis registry.Discover) *Picker {
	return &Picker{
		cc:       cc,
		locator:  loc,
		discover: dis,
		subConns: make(map[string]balancer.SubConn),
		svcIndex: make(map[string]*serviceNodes),
	}
}

func (p *Picker) updateIndex(instances map[string]endpoint.ServiceInstance) {
	newIndex := make(map[string]*serviceNodes)

	for _, instance := range instances {
		for _, rpcService := range instance.RpcServices {
			svc := newIndex[rpcService.Name]
			if svc == nil {
				svc = &serviceNodes{}
				newIndex[rpcService.Name] = svc
			}
			svc.nodes = append(svc.nodes, &wrrNode{instance: instance})
		}
	}

	p.mu.Lock()
	p.svcIndex = newIndex
	p.mu.Unlock()
}

func (p *Picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	strat := lx.GetStrategy(info.Ctx)
	addr, err := p.resolve(info.Ctx, strat)
	if err != nil {
		return balancer.PickResult{}, err
	}
	sc := p.getOrCreateSubConn(addr)
	return balancer.PickResult{SubConn: sc}, nil
}

func (p *Picker) resolve(ctx context.Context, strat lx.Strategy) (string, error) {
	switch strat.Mode {
	case lx.ModeSelect:
		nodeId, err := p.locator.Locate(ctx, strat.Name, strat.Key)
		if err != nil {
			return "", err
		}
		instance, ok, err := p.discover.Get(ctx, nodeId)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrServerNotFound
		}
		return instance.RpcTarget(), nil

	case lx.ModeNode:
		instance, ok, err := p.discover.Get(ctx, strat.Key)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrServerNotFound
		}
		return instance.RpcTarget(), nil

	case lx.ModeBalance:
		return p.pickBalanced(strat.Name)

	case lx.ModeDirect:
		return strat.Addr, nil

	default:
		return "", ErrUnknowStrategy
	}
}

func (p *Picker) pickBalanced(serviceName string) (string, error) {
	p.mu.RLock()
	svc := p.svcIndex[serviceName]
	p.mu.RUnlock()

	if svc == nil {
		return "", ErrServerNotFound
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	var (
		selected    *wrrNode
		totalWeight int
	)
	for _, node := range svc.nodes {
		if node.instance.Status != endpoint.Working {
			continue
		}
		node.currentWeight += node.instance.Weight
		totalWeight += node.instance.Weight
		if selected == nil || node.currentWeight > selected.currentWeight {
			selected = node
		}
	}

	if selected == nil {
		return "", ErrServerNotFound
	}
	selected.currentWeight -= totalWeight
	return selected.instance.RpcTarget(), nil
}

func (p *Picker) getOrCreateSubConn(addr string) balancer.SubConn {
	p.mu.RLock()
	sc, ok := p.subConns[addr]
	p.mu.RUnlock()
	if ok {
		return sc
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if sc, ok := p.subConns[addr]; ok {
		return sc
	}
	sc, _ = p.cc.NewSubConn([]resolver.Address{{Addr: addr}}, balancer.NewSubConnOptions{})
	p.subConns[addr] = sc
	sc.Connect()
	return sc
}
