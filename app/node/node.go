package node

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/2comjie/nova/app"
	"github.com/2comjie/nova/core/endpoint"
	pbGate "github.com/2comjie/nova/internal/pb/transport/gate"
	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network"
	"github.com/2comjie/nova/registry"
	"github.com/2comjie/nova/rpc"
	"github.com/2comjie/nova/rpc/lx"
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
	RPCServer   *rpc.Server
	RPCListener net.Listener
	Components  []app.Component
}

type Node struct {
	pbNode.UnimplementedNodeServer
	*app.App

	instance    endpoint.ServiceInstance
	router      *Router
	nodeLocator *locator.NodeLocator
	gateLocator *locator.GateLocator
	gateClient  pbGate.GateClient
	registry    registry.Registry
	discovery   registry.Discover
	rpcServer   *rpc.Server
	rpcListener net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
	started     atomic.Bool
	closed      atomic.Bool
	serverWait  sync.WaitGroup
	wait        sync.WaitGroup
}

func New(config Config) *Node {
	if config.Instance.Id == "" || config.Instance.ServiceName == "" || config.Instance.ServiceName == locator.GateName {
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
		panic("node: 必须提供TCP RPC Server")
	}
	if config.RPCListener == nil {
		panic("node: 必须提供TCP RPC Listener")
	}
	if config.Discovery == nil {
		panic("node: 必须提供Discovery")
	}
	ctx, cancel := context.WithCancel(context.Background())
	node := &Node{
		App:         app.New(config.Components...),
		instance:    config.Instance,
		router:      config.Router,
		nodeLocator: config.NodeLocator,
		gateLocator: config.GateLocator,
		gateClient:  config.GateClient,
		registry:    config.Registry,
		discovery:   config.Discovery,
		rpcServer:   config.RPCServer,
		rpcListener: config.RPCListener,
		ctx:         ctx,
		cancel:      cancel,
	}
	pbNode.RegisterNodeServer(config.RPCServer, node)
	return node
}

func (n *Node) Router() *Router {
	return n.router
}

func (n *Node) Start() error {
	if n.closed.Load() {
		return ErrClosed
	}
	if !n.started.CompareAndSwap(false, true) {
		return ErrStarted
	}
	if err := n.App.Start(); err != nil {
		n.cancel()
		n.closed.Store(true)
		return err
	}
	if err := n.registry.Register(n.instance); err != nil {
		n.cancel()
		_ = n.App.Shutdown(context.Background())
		n.Wait()
		n.closed.Store(true)
		return err
	}

	n.serverWait.Add(1)
	go func() {
		defer n.serverWait.Done()
		if err := n.rpcServer.Serve(n.rpcListener); err != nil && !n.closed.Load() {
			logx.Errorf("node: TCP RPC服务退出: %v", err)
		}
	}()
	return nil
}

func (n *Node) Shutdown(ctx context.Context) error {
	if !n.closed.CompareAndSwap(false, true) {
		return nil
	}

	if n.started.Load() {
		_ = n.registry.Deregister(n.instance.Id)
	}

	n.App.RequestStop()
	_ = n.rpcServer.Shutdown(ctx)
	n.serverWait.Wait()
	n.cancel()

	componentErr := n.App.Shutdown(ctx)

	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		n.Wait()
	}()
	select {
	case <-waitDone:
	case <-ctx.Done():
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return componentErr
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
	digest := hmac.New(sha256.New, []byte(n.instance.Id))
	_, _ = digest.Write([]byte(rand.Text()))
	return hex.EncodeToString(digest.Sum(nil)[:16])
}

func (n *Node) UpdateMetadata(metadata map[string]string) error {
	err := n.registry.UpdateMetaData(n.instance.Id, metadata)
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
	err := n.registry.DeleteMetaData(n.instance.Id, keys)
	if err != nil {
		return err
	}
	for _, key := range keys {
		delete(n.instance.MetaData, key)
	}
	return nil
}

func (n *Node) Push(ctx context.Context, uid uint64, route uint32, body []byte) error {
	gateId, err := n.gateLocator.Locate(ctx, uid)
	if err != nil {
		return err
	}
	if gateId == "" {
		return network.ErrNotBound
	}
	_, err = n.gateClient.Push(lx.WithNode(ctx, gateId), &pbGate.PushRequest{
		Uid:             uid,
		Route:           route,
		Body:            body,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.Id,
	})
	return err
}

func (n *Node) Kick(ctx context.Context, uid uint64) error {
	binding, err := n.gateLocator.LocateBinding(ctx, uid)
	if err != nil {
		return err
	}
	if binding.InstanceId == "" {
		return nil
	}
	_, err = n.gateClient.Kick(lx.WithNode(ctx, binding.InstanceId), &pbGate.KickRequest{
		Uid:             uid,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.Id,
		SessionId:       binding.SessionId,
	})
	return err
}

// Broadcast returns the delivered count from completed Gate calls, stopping at the first error.
func (n *Node) Broadcast(ctx context.Context, route uint32, body []byte) (uint32, error) {
	instances, err := n.discovery.List(ctx)
	if err != nil {
		return 0, err
	}
	var count uint32
	for _, instance := range instances {
		if instance.ServiceName != locator.GateName {
			continue
		}
		response, err := n.gateClient.Broadcast(lx.WithNode(ctx, instance.Id), &pbGate.BroadcastRequest{
			Route:           route,
			Body:            body,
			NodeServiceName: n.instance.ServiceName,
			NodeInstanceId:  n.instance.Id,
		})
		if err != nil {
			return count, err
		}
		count += response.Count
	}
	return count, nil
}

// MultiPush skips offline users and stops at the first error, returning completed Gate counts.
func (n *Node) MultiPush(ctx context.Context, uidList []uint64, route uint32, body []byte) (uint32, error) {
	groups := make(map[string][]uint64)
	for _, uid := range uidList {
		gateId, err := n.gateLocator.Locate(ctx, uid)
		if err != nil {
			return 0, err
		}
		if gateId != "" {
			groups[gateId] = append(groups[gateId], uid)
		}
	}
	var count uint32
	for gateId, uids := range groups {
		response, err := n.gateClient.MultiPush(lx.WithNode(ctx, gateId), &pbGate.MultiPushRequest{
			UidList:         uids,
			Route:           route,
			Body:            body,
			NodeServiceName: n.instance.ServiceName,
			NodeInstanceId:  n.instance.Id,
		})
		if err != nil {
			return count, err
		}
		count += response.Count
	}
	return count, nil
}

func (n *Node) MockGateCall(ctx context.Context, uid uint64, route uint32, body []byte) ([]byte, bool, error) {
	response, err := n.gateClient.MockCall(lx.WithBalance(ctx, locator.GateName), &pbGate.MockCallRequest{
		Uid:             uid,
		Route:           route,
		Body:            body,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.Id,
	})
	if err != nil {
		return nil, false, err
	}
	return response.Body, response.Replied, nil
}

func (n *Node) ListServices(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	serviceList, err := n.discovery.List(ctx)
	if err != nil {
		return nil, err
	}
	return serviceList, nil
}
