package node

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	ErrStarted = errors.New("node: Node已经启动")
	ErrClosed  = errors.New("node: Node已经关闭")
)

type Config struct {
	Instance    endpoint.ServiceInstance
	Router      *Router
	NodeLocator *locator.NodeLocator
	GateLocator *locator.GateLocator
	GateClient  pbGate.GateClient
	Registry    registry.Registry
	Discovery   registry.Discover
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
	discovery   registry.Discover
	rpcServer   *grpc.Server
	rpcListener net.Listener
	components  []app.Component
	componentMu sync.Mutex

	ctx               context.Context
	cancel            context.CancelFunc
	started           atomic.Bool
	closed            atomic.Bool
	startedComponents atomic.Int64
	serverWait        sync.WaitGroup
	wait              sync.WaitGroup
}

func New(config Config) *Node {
	if config.Instance.ID == "" || config.Instance.ServiceName == "" || config.Instance.ServiceName == locator.GateName {
		panic("node: 必须提供Node ServiceInstance")
	}
	if config.Router == nil {
		panic("node: 必须提供Router")
	}
	if config.NodeLocator == nil {
		panic("node: 必须提供NodeLocator")
	}
	if config.GateLocator == nil {
		panic("node: 必须提供GateLocator")
	}
	if config.GateClient == nil {
		panic("node: 必须提供GateClient")
	}
	if config.Registry == nil {
		panic("node: 必须提供Registry")
	}
	if config.RPCServer == nil {
		panic("node: 必须提供gRPC Server")
	}
	if config.RPCListener == nil {
		panic("node: 必须提供gRPC Listener")
	}
	if config.Discovery == nil {
		panic("node: 必须提供Discovery")
	}
	if err := config.Router.Freeze(); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	node := &Node{
		instance:    config.Instance,
		router:      config.Router,
		nodeLocator: config.NodeLocator,
		gateLocator: config.GateLocator,
		gateClient:  config.GateClient,
		registry:    config.Registry,
		discovery:   config.Discovery,
		rpcServer:   config.RPCServer,
		rpcListener: config.RPCListener,
		components:  append([]app.Component(nil), config.Components...),
		ctx:         ctx,
		cancel:      cancel,
	}
	pbNode.RegisterNodeServer(config.RPCServer, node)
	return node
}

func (n *Node) AddComponent(component app.Component) error {
	n.componentMu.Lock()
	defer n.componentMu.Unlock()
	if n.closed.Load() {
		return ErrClosed
	}
	if n.started.Load() {
		return ErrStarted
	}
	n.components = append(n.components, component)
	return nil
}

func (n *Node) Start() error {
	n.componentMu.Lock()
	if n.closed.Load() {
		n.componentMu.Unlock()
		return ErrClosed
	}
	if !n.started.CompareAndSwap(false, true) {
		n.componentMu.Unlock()
		return ErrStarted
	}
	n.componentMu.Unlock()

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

func (n *Node) RandString() string {
	digest := hmac.New(sha256.New, []byte(n.instance.ID))
	_, _ = digest.Write([]byte(rand.Text()))
	return hex.EncodeToString(digest.Sum(nil)[:16])
}

func (n *Node) UpdateMetadata(metadata map[string]string) error {
	err := n.registry.UpdateMetaData(n.instance.ID, metadata)
	if err != nil {
		return err
	}
	if n.instance.MetaData == nil {
		n.instance.MetaData = make(map[string]string)
	}
	for key, value := range metadata {
		n.instance.MetaData[key] = value
	}
	return nil
}

func (n *Node) DeleteMetadata(keys ...string) error {
	err := n.registry.DeleteMetaData(n.instance.ID, keys)
	if err != nil {
		return err
	}
	for _, key := range keys {
		delete(n.instance.MetaData, key)
	}
	return nil
}

func (n *Node) Push(ctx context.Context, uid string, route uint32, body []byte) error {
	_, err := n.gateClient.Push(ctx, &pbGate.PushRequest{
		Uid:             uid,
		Route:           route,
		Body:            body,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.ID,
	})
	return err
}
func (n *Node) Kick(ctx context.Context, uid string) error {
	_, err := n.gateClient.Kick(ctx, &pbGate.KickRequest{
		Uid:             uid,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.ID,
	})
	return err
}
func (n *Node) Broadcast(ctx context.Context, route uint32, body []byte) (uint32, error) {
	response, err := n.gateClient.Broadcast(ctx, &pbGate.BroadcastRequest{
		Route:           route,
		Body:            body,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.ID,
	})
	if err != nil {
		return 0, err
	}
	return response.Count, nil
}
func (n *Node) MultiPush(ctx context.Context, uidList []string, route uint32, body []byte) (uint32, error) {
	response, err := n.gateClient.MultiPush(ctx, &pbGate.MultiPushRequest{
		UidList:         uidList,
		Route:           route,
		Body:            body,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.ID,
	})
	if err != nil {
		return 0, err
	}
	return response.Count, nil
}
func (n *Node) ListServices(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	serviceList, err := n.discovery.List(ctx)
	if err != nil {
		return nil, err
	}
	return serviceList, nil
}
