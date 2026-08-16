package client

import (
	"context"
	"net"
	"sync"

	"github.com/2comjie/nova/core/endpoint"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/registry"
	"github.com/2comjie/nova/rpc/lx"
	"github.com/cespare/xxhash/v2"
	"google.golang.org/grpc"
)

type Client struct {
	discover  registry.Discover
	locator   locator.Locator
	pool      *ConnPool
	balancers map[lx.BalancePolicy]Balancer

	mu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	serviceMap  map[string][]endpoint.ServiceInstance
	serviceAddr map[string]struct{}
}

func NewClient(discover registry.Discover, locator locator.Locator, opts ...Option) *Client {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		discover:    discover,
		locator:     locator,
		pool:        NewConnPool(options.dialOptions...),
		balancers:   options.balancers,
		ctx:         ctx,
		cancel:      cancel,
		serviceMap:  make(map[string][]endpoint.ServiceInstance),
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
	instance, err := c.pickService(ctx, serviceName, policy)
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

func (c *Client) Route(
	ctx context.Context,
	serviceName string,
	binding string,
	key string,
) (*grpc.ClientConn, error) {
	if serviceName == "" || binding == "" || key == "" {
		return nil, ErrInvalidTarget
	}
	if c.locator == nil {
		return nil, ErrLocatorUnavailable
	}
	instanceID, err := c.locator.Locate(ctx, binding, key)
	if err != nil {
		return nil, err
	}
	return c.Node(ctx, serviceName, instanceID)
}

func (c *Client) Actor(ctx context.Context, serviceName string, actorKey string) (*grpc.ClientConn, error) {
	instance, err := c.pickActor(ctx, serviceName, actorKey)
	if err != nil {
		return nil, err
	}
	return c.pool.Get(instance.RpcTarget())
}

func (c *Client) ActorInstanceId(ctx context.Context, serviceName string, actorKey string) (string, error) {
	instance, err := c.pickActor(ctx, serviceName, actorKey)
	if err != nil {
		return "", err
	}
	return instance.ID, nil
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
		return c.Route(ctx, s.Service, s.Binding, s.Key)
	case lx.ModeActor:
		return c.Actor(ctx, s.Service, s.Key)
	case lx.ModeBalance:
		return c.Service(ctx, s.Service)
	default:
		return c.Service(ctx, s.Service)
	}
}

func (c *Client) pickActor(ctx context.Context, serviceName string, actorKey string) (endpoint.ServiceInstance, error) {
	if err := ctx.Err(); err != nil {
		return endpoint.ServiceInstance{}, err
	}
	if serviceName == "" || actorKey == "" {
		return endpoint.ServiceInstance{}, ErrInvalidTarget
	}

	c.mu.RLock()
	instances := c.serviceMap[serviceName]
	if len(instances) == 0 {
		c.mu.RUnlock()
		return endpoint.ServiceInstance{}, ErrNoAnyService
	}
	selected := instances[0]
	selectedScore := xxhash.Sum64String(actorKey + "\x00" + selected.ID)
	for index := 1; index < len(instances); index++ {
		instance := instances[index]
		score := xxhash.Sum64String(actorKey + "\x00" + instance.ID)
		if score > selectedScore || score == selectedScore && instance.ID < selected.ID {
			selected = instance
			selectedScore = score
		}
	}
	c.mu.RUnlock()
	return selected, nil
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
	serviceMap := make(map[string][]endpoint.ServiceInstance)
	activeAddr := make(map[string]struct{})
	for _, instance := range instances {
		if instance.Status != endpoint.Working || instance.ServiceName == "" {
			continue
		}
		if instance.RpcHost == "" || instance.RpcPort <= 0 {
			continue
		}
		serviceMap[instance.ServiceName] = append(serviceMap[instance.ServiceName], instance)
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

func (c *Client) pickService(
	ctx context.Context,
	serviceName string,
	policy lx.BalancePolicy,
) (endpoint.ServiceInstance, error) {
	c.mu.RLock()
	instances := append([]endpoint.ServiceInstance(nil), c.serviceMap[serviceName]...)
	c.mu.RUnlock()
	if len(instances) == 0 {
		return endpoint.ServiceInstance{}, ErrNoAnyService
	}

	balancer := c.balancers[policy]
	if balancer == nil {
		return endpoint.ServiceInstance{}, ErrInvalidBalancePolicy
	}
	return balancer.Pick(ctx, serviceName, instances)
}
