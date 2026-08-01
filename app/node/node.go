package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/app"
	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/registry"
	"google.golang.org/grpc"
)

var (
	ErrInstanceRequired    = errors.New("node: 必须提供Node ServiceInstance")
	ErrRouterRequired      = errors.New("node: 必须提供Router")
	ErrNodeLocatorRequired = errors.New("node: 必须提供NodeLocator")
	ErrGateLocatorRequired = errors.New("node: 必须提供GateLocator")
	ErrGateClientRequired  = errors.New("node: 必须提供GateClient")
	ErrRegistryRequired    = errors.New("node: 必须提供Registry")
	ErrRPCServerRequired   = errors.New("node: 必须提供gRPC Server")
	ErrRPCListenerRequired = errors.New("node: 必须提供gRPC Listener")
	ErrStarted             = errors.New("node: Node已经启动")
	ErrClosed              = errors.New("node: Node已经关闭")
)

type Config struct {
	Instance    endpoint.ServiceInstance
	Router      *Router
	NodeLocator *locator.NodeLocator
	GateLocator *locator.GateLocator
	GateClient  pbGate.GateClient
	Registry    registry.Registry
	RPCServer   *grpc.Server
	RPCListener net.Listener
	Components  []app.Component
}

type Node struct {
	pbNode.UnimplementedNodeServer

	instance    endpoint.ServiceInstance
	router      *Router
	nodeLocator *locator.NodeLocator
	gateLocator *locator.GateLocator
	gateClient  pbGate.GateClient
	registry    registry.Registry
	rpcServer   *grpc.Server
	rpcListener net.Listener
	components  []app.Component
	proxy       *Proxy

	ctx               context.Context
	cancel            context.CancelFunc
	started           atomic.Bool
	closed            atomic.Bool
	startedComponents atomic.Int64
	serverWait        sync.WaitGroup
	wait              sync.WaitGroup
}

func New(config Config) (*Node, error) {
	if config.Instance.ID == "" ||
		config.Instance.ServiceName == "" ||
		config.Instance.ServiceName == locator.GateName {
		return nil, ErrInstanceRequired
	}
	if config.Router == nil {
		return nil, ErrRouterRequired
	}
	if config.NodeLocator == nil {
		return nil, ErrNodeLocatorRequired
	}
	if config.GateLocator == nil {
		return nil, ErrGateLocatorRequired
	}
	if config.GateClient == nil {
		return nil, ErrGateClientRequired
	}
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}
	if config.RPCServer == nil {
		return nil, ErrRPCServerRequired
	}
	if config.RPCListener == nil {
		return nil, ErrRPCListenerRequired
	}
	if err := config.Router.Freeze(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	node := &Node{
		instance:    config.Instance,
		router:      config.Router,
		nodeLocator: config.NodeLocator,
		gateLocator: config.GateLocator,
		gateClient:  config.GateClient,
		registry:    config.Registry,
		rpcServer:   config.RPCServer,
		rpcListener: config.RPCListener,
		components:  append([]app.Component(nil), config.Components...),
		ctx:         ctx,
		cancel:      cancel,
	}
	node.proxy = &Proxy{app: node}
	pbNode.RegisterNodeServer(config.RPCServer, node)
	return node, nil
}

func (n *Node) Start() error {
	if n.closed.Load() {
		return ErrClosed
	}
	if !n.started.CompareAndSwap(false, true) {
		return ErrStarted
	}

	for _, component := range n.components {
		logx.Infof("node: 正在启动组件 name=%s", component.Name())
		if err := component.Start(); err != nil {
			var errs []error
			errs = append(errs, fmt.Errorf("node: 启动组件失败 name=%s: %w", component.Name(), err))
			n.cancel()
			for index := int(n.startedComponents.Load()) - 1; index >= 0; index-- {
				startedComponent := n.components[index]
				logx.Infof("node: 正在回滚组件 name=%s", startedComponent.Name())
				if shutdownErr := startedComponent.Shutdown(context.Background()); shutdownErr != nil {
					errs = append(errs, fmt.Errorf(
						"node: 回滚组件失败 name=%s: %w",
						startedComponent.Name(),
						shutdownErr,
					))
				}
			}
			n.Wait()
			n.closed.Store(true)
			return errors.Join(errs...)
		}
		n.startedComponents.Add(1)
		logx.Infof("node: 组件启动完成 name=%s", component.Name())
	}
	if err := n.registry.Register(n.instance); err != nil {
		var errs []error
		errs = append(errs, err)
		n.cancel()
		for index := int(n.startedComponents.Load()) - 1; index >= 0; index-- {
			component := n.components[index]
			logx.Infof("node: 正在回滚组件 name=%s", component.Name())
			if shutdownErr := component.Shutdown(context.Background()); shutdownErr != nil {
				errs = append(errs, fmt.Errorf(
					"node: 回滚组件失败 name=%s: %w",
					component.Name(),
					shutdownErr,
				))
			}
		}
		n.Wait()
		n.closed.Store(true)
		return errors.Join(errs...)
	}

	n.serverWait.Add(1)
	help.SafeGo(func() {
		defer n.serverWait.Done()
		if err := n.rpcServer.Serve(n.rpcListener); err != nil && !n.closed.Load() {
			logx.Errorf("node: gRPC服务退出: %v", err)
		}
	})
	return nil
}

func (n *Node) Shutdown(ctx context.Context) error {
	if !n.closed.CompareAndSwap(false, true) {
		return nil
	}

	var errs []error
	if n.started.Load() {
		if err := n.registry.Deregister(n.instance.ID); err != nil {
			errs = append(errs, err)
		}
	}

	rpcDone := make(chan struct{})
	help.SafeGo(func() {
		defer close(rpcDone)
		n.rpcServer.GracefulStop()
	})
	select {
	case <-rpcDone:
	case <-ctx.Done():
		n.rpcServer.Stop()
		<-rpcDone
		errs = append(errs, ctx.Err())
	}
	n.serverWait.Wait()
	n.cancel()

	for index := int(n.startedComponents.Load()) - 1; index >= 0; index-- {
		component := n.components[index]
		logx.Infof("node: 正在关闭组件 name=%s", component.Name())
		if err := component.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("node: 关闭组件失败 name=%s: %w", component.Name(), err))
			continue
		}
		logx.Infof("node: 组件关闭完成 name=%s", component.Name())
	}

	waitDone := make(chan struct{})
	help.SafeGo(func() {
		defer close(waitDone)
		n.Wait()
	})
	select {
	case <-waitDone:
	case <-ctx.Done():
		errs = append(errs, ctx.Err())
	}
	return errors.Join(errs...)
}

func (n *Node) AddWait() {
	n.wait.Add(1)
}

func (n *Node) DoneWait() {
	n.wait.Done()
}

func (n *Node) Wait() {
	n.wait.Wait()
}

func (n *Node) Done() <-chan struct{} {
	return n.ctx.Done()
}
