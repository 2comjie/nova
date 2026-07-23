package client

import (
	"context"
	"net"
	"sync"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/registry"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
)

type Client struct {
	discover registry.Discover
	locator  locator.Locator
	pool     *ConnPool

	mu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	serviceMap  map[string][]*weightInstance
	nextSeq     map[string]uint64
	serviceAddr map[string]struct{}
}

type weightInstance struct {
	endpoint.ServiceInstance
	curWeight int
}

func NewClient(discover registry.Discover, locator locator.Locator, opts ...grpc.DialOption) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		discover:    discover,
		locator:     locator,
		pool:        NewConnPool(opts...),
		ctx:         ctx,
		cancel:      cancel,
		serviceMap:  make(map[string][]*weightInstance),
		nextSeq:     make(map[string]uint64),
		serviceAddr: make(map[string]struct{}),
	}

	if instances, err := discover.List(ctx); err != nil {
		logx.Errorf("rpc client discover.List() failed: %v", err)
	} else {
		c.update(instances)
	}
	help.SafeGo(c.watch)
	return c
}

func (c *Client) Service(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if serviceName == "" {
		return nil, ErrInvalidTarget
	}

	policy := lx.GetStrategy(ctx).BalancePolicy
	instance, err := c.pickService(serviceName, policy)
	if err != nil {
		return nil, err
	}
	return c.pool.Get(instance.RpcTarget())
}

func (c *Client) Node(ctx context.Context, serviceName, instanceID string) (*grpc.ClientConn, error) {
	if instanceID == "" || serviceName == "" {
		return nil, ErrInvalidTarget
	}
	instance, ok, err := c.discover.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if !ok || instance.ServiceName != serviceName {
		return nil, ErrNoAnyService
	}
	return c.pool.Get(instance.RpcTarget())
}

func (c *Client) Direct(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, ErrInvalidTarget
	}
	return c.pool.Get(addr)
}

func (c *Client) Route(ctx context.Context, serviceName, key string) (*grpc.ClientConn, error) {
	if serviceName == "" || key == "" {
		return nil, ErrInvalidTarget
	}
	if c.locator == nil {
		return nil, ErrLocatorUnavailable
	}
	instanceID, err := c.locator.Locate(ctx, serviceName, key)
	if err != nil {
		return nil, err
	}
	return c.Node(ctx, serviceName, instanceID)
}

func (c *Client) Conn(ctx context.Context) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := lx.GetStrategy(ctx)
	switch s.Mode {
	case lx.ModeDirect:
		return c.Direct(ctx, s.Addr)
	case lx.ModeNode:
		instance, ok, err := c.discover.Get(ctx, s.Key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrNoAnyService
		}
		return c.pool.Get(instance.RpcTarget())
	case lx.ModeSelect:
		return c.Route(ctx, s.Name, s.Key)
	case lx.ModeBalance:
		return c.Service(ctx, s.Name)
	default:
		return c.Service(ctx, s.Name)
	}
}

func (c *Client) Close() {
	c.cancel()
	c.pool.Close()
}

func (c *Client) watch() {
	for {
		instances, err := c.discover.Next(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			logx.Errorf("rpc client discover.Next() failed: %v", err)
			continue
		}
		c.update(instances)
	}
}

func (c *Client) update(instances map[string]endpoint.ServiceInstance) {
	serviceMap := make(map[string][]*weightInstance)
	activeAddr := make(map[string]struct{})
	for _, instance := range instances {
		if instance.Status != endpoint.Working || instance.ServiceName == "" {
			continue
		}
		if instance.RpcHost == "" || instance.RpcPort <= 0 {
			continue
		}
		serviceMap[instance.ServiceName] = append(
			serviceMap[instance.ServiceName],
			&weightInstance{ServiceInstance: instance},
		)
		activeAddr[instance.RpcTarget()] = struct{}{}
	}

	c.mu.Lock()
	staleAddr := make(map[string]bool)
	for addr := range c.serviceAddr {
		if _, ok := activeAddr[addr]; !ok {
			staleAddr[addr] = true
		}
	}
	c.serviceMap = serviceMap
	c.serviceAddr = activeAddr
	c.mu.Unlock()

	c.pool.Remove(staleAddr)
}

func (c *Client) pickService(serviceName string, policy lx.BalancePolicy) (endpoint.ServiceInstance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	instances := c.serviceMap[serviceName]
	if len(instances) == 0 {
		return endpoint.ServiceInstance{}, ErrNoAnyService
	}

	switch policy {
	case lx.BalanceRoundRobin:
		seq := c.nextSeq[serviceName]
		c.nextSeq[serviceName] = seq + 1
		return instances[seq%uint64(len(instances))].ServiceInstance, nil
	case lx.BalanceWeightedRoundRobin:
		return pickWeighted(instances), nil
	default:
		return endpoint.ServiceInstance{}, ErrInvalidBalancePolicy
	}
}

func pickWeighted(instances []*weightInstance) endpoint.ServiceInstance {
	var selected *weightInstance
	totalWeight := 0
	for _, instance := range instances {
		weight := instance.Weight
		if weight <= 0 {
			weight = 1
		}
		instance.curWeight += weight
		totalWeight += weight
		if selected == nil || instance.curWeight > selected.curWeight {
			selected = instance
		}
	}
	if selected == nil {
		return endpoint.ServiceInstance{}
	}
	selected.curWeight -= totalWeight
	return selected.ServiceInstance
}
