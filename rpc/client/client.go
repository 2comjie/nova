package client

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/registry"
	"google.golang.org/grpc"
)

type Client struct {
	discover    registry.Discover
	locator     locator.Locator
	pool        *ConnPool
	dialOptions []grpc.DialOption
	nextSeq     atomic.Uint64

	ctx    context.Context
	cancel context.CancelFunc

	mu                    sync.RWMutex
	rpcServiceInstanceMap map[string][]string // service -> serviceID
}

func NewClient(discover registry.Discover, locator locator.Locator, opts ...grpc.DialOption) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	cl := &Client{
		discover:              discover,
		locator:               locator,
		pool:                  NewConnPool(opts...),
		dialOptions:           opts,
		nextSeq:               atomic.Uint64{},
		ctx:                   ctx,
		cancel:                cancel,
		mu:                    sync.RWMutex{},
		rpcServiceInstanceMap: make(map[string][]string),
	}
	// 服务启动的时候 直接拉取一次
	list, err := discover.List(ctx)
	if err != nil {
		logx.Errorf("rpc client discover.List() failed: %v", err)
	} else {
		cl.updateRpcServices(list)
	}
	help.SafeGo(cl.watch)
	return cl
}

func (c *Client) Select(ctx context.Context, routeName string, key string) (*grpc.ClientConn, error) {
	target, err := c.locator.Locate(ctx, routeName, key)
	if err != nil {
		return nil, err
	}
	return c.Node(ctx, target)
}

func (c *Client) Service(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	target, err := c.pickService(serviceName)
	if err != nil {
		return nil, err
	}
	return c.Node(ctx, target)
}

func (c *Client) Node(ctx context.Context, instanceID string) (*grpc.ClientConn, error) {
	target, ok, err := c.discover.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrServiceNotFound
	}
	return c.Direct(ctx, target.RpcTarget())
}

func (c *Client) Direct(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, ErrInvalidTarget
	}
	return c.pool.Get(addr)
}

func (c *Client) pickService(serviceName string) (string, error) {
	var serviceList []string
	c.mu.RLock()
	serviceList = c.rpcServiceInstanceMap[serviceName]
	c.mu.RUnlock()
	if len(serviceList) == 0 {
		return "", ErrServiceNotFound
	}
	seq := c.nextSeq.Add(1)
	targetService := serviceList[seq%uint64(len(serviceList))]
	return targetService, nil
}

func (c *Client) extractRpcService(instanceList map[string]endpoint.ServiceInstance) map[string][]string {
	rpcServiceMap := make(map[string][]string)
	for _, serviceInstance := range instanceList {
		for _, rpcService := range serviceInstance.RpcServices {
			rpcServiceMap[rpcService.Name] = append(rpcServiceMap[rpcService.Name], serviceInstance.ID)
		}
	}
	return rpcServiceMap
}

func (c *Client) watch() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			list, err := c.discover.Next(c.ctx)
			if err != nil {
				logx.Errorf("rpc client discover.Next() failed: %v", err)
				continue
			}
			c.updateRpcServices(list)
		}
	}
}

func (c *Client) updateRpcServices(instanceList map[string]endpoint.ServiceInstance) {
	rpcServiceMap := c.extractRpcService(instanceList)
	activeAddrs := make(map[string]bool, len(instanceList))
	for _, inst := range instanceList {
		activeAddrs[inst.RpcTarget()] = true
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.pool.Prune(activeAddrs)
	c.rpcServiceInstanceMap = rpcServiceMap // 服务更新
	logx.Debugf("rpc client updateRpcServices: %v", rpcServiceMap)
}
