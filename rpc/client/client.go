package client

import (
	"context"
	"net"
	"sync"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/registry"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Client struct {
	discover registry.Discover
	locator  locator.Locator
	pool     *ConnPool

	dialOpts []grpc.DialOption
	mu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	rpcServiceInstanceMap map[string][]RpcServiceInstance
	nextSeq               map[string]uint64
}

func NewClient(discover registry.Discover, locator locator.Locator, opts ...grpc.DialOption) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	cl := &Client{
		discover:              discover,
		locator:               locator,
		pool:                  NewConnPool(opts...),
		dialOpts:              opts,
		mu:                    sync.RWMutex{},
		ctx:                   ctx,
		cancel:                cancel,
		rpcServiceInstanceMap: make(map[string][]RpcServiceInstance),
		nextSeq:               make(map[string]uint64),
	}

	help.SafeGo(func() {
		cl.watch()
	})

	return cl
}

type RpcServiceInstance struct {
	rpcService      endpoint.RpcService
	serviceInstance endpoint.ServiceInstance
}

func (c *Client) Service(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	rpcServiceInstance, err := c.pickService(serviceName)
	if err != nil {
		return nil, err
	}
	return c.pool.Get(rpcServiceInstance.rpcService.Target())
}

func (c *Client) Node(ctx context.Context, serviceName string, instanceID string) (*grpc.ClientConn, error) {
	if instanceID == "" {
		return nil, ErrInvalidTarget
	}
	serviceInstance, err := c.discover.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	rpcService := serviceInstance.RpcServices[serviceName]
	return c.pool.Get(rpcService.Target())
}

func (c *Client) Direct(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	_ = ctx
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, ErrInvalidTarget
	}
	return c.pool.Get(addr)
}

func (c *Client) Route(ctx context.Context, serviceName string, key string) (*grpc.ClientConn, error) {
	if key == "" {
		return nil, ErrInvalidTarget
	}
	instanceId, err := c.locator.Locate(ctx, serviceName, key)
	if err != nil {
		return nil, err
	}
	return c.Node(ctx, serviceName, instanceId)
}

func (c *Client) Close() {
	c.cancel()
	_ = c.pool.Close()
}

func (c *Client) watch() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		list, err := c.discover.Next(c.ctx)
		if err != nil {
			zap.S().Errorf("rpc client discover.Next() failed: %v", err)
			continue
		}

		c.mu.Lock()
		c.rpcServiceInstanceMap = c.extractRpcService(list)
		activeAddrs := make(map[string]bool)
		for _, instances := range c.rpcServiceInstanceMap {
			for _, inst := range instances {
				activeAddrs[inst.rpcService.Target()] = true
			}
		}
		c.mu.Unlock()
		c.pool.Prune(activeAddrs)
		zap.S().Debugf("rpc client watch: %v", c.rpcServiceInstanceMap)
	}
}

func (c *Client) pickService(serviceName string) (RpcServiceInstance, error) {
	var serviceList []RpcServiceInstance
	c.mu.RLock()
	serviceList = c.rpcServiceInstanceMap[serviceName]
	c.mu.RUnlock()
	if len(serviceList) == 0 {
		return RpcServiceInstance{}, ErrServiceNotFound
	}
	c.mu.Lock()
	seq := c.nextSeq[serviceName]
	c.nextSeq[serviceName] = seq + 1
	c.mu.Unlock()
	rpcService := serviceList[seq%uint64(len(serviceList))]
	return rpcService, nil
}

func (c *Client) extractRpcService(instanceList map[string]endpoint.ServiceInstance) map[string][]RpcServiceInstance {
	rpcServiceMap := make(map[string][]RpcServiceInstance)
	for _, serviceInstance := range instanceList {
		if serviceInstance.Status == endpoint.Work {
			for _, rpcService := range serviceInstance.RpcServices {
				rpcServiceMap[rpcService.Name] = append(rpcServiceMap[rpcService.Name], RpcServiceInstance{
					rpcService:      rpcService,
					serviceInstance: serviceInstance,
				})
			}
		}
	}
	return rpcServiceMap
}
