package gate

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/nova/app"
	"github.com/2comjie/nova/core/endpoint"
	"github.com/2comjie/nova/core/help"
	pbGate "github.com/2comjie/nova/internal/pb/transport/gate"
	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network"
	"github.com/2comjie/nova/registry"
	"github.com/2comjie/nova/rpc"
	"github.com/2comjie/nova/rpc/lx"
)

const defaultLocatorTimeout = 3 * time.Second

var (
	ErrStarted           = errors.New("gate: Gate已经启动")
	ErrClosed            = errors.New("gate: Gate已经关闭")
	ErrInvalidNodeSource = errors.New("gate: Node来源信息无效")
)

type ErrorHandler func(ctx *Context, err error)

type Config struct {
	Instance       endpoint.ServiceInstance
	Router         *Router
	NodeClient     pbNode.NodeClient
	GateClient     pbGate.GateClient
	Locator        *locator.GateLocator
	Registry       registry.Registry
	RPCServer      *rpc.Server
	RPCListener    net.Listener
	NetworkOptions []network.Option
	Hooks          network.Hooks
	ErrorHandler   ErrorHandler
	LocatorTimeout time.Duration
	Components     []app.Component
}

type Gate struct {
	pbGate.UnimplementedGateServer
	*app.App

	instance       endpoint.ServiceInstance
	router         *Router
	server         *network.Server
	nodeClient     pbNode.NodeClient
	gateClient     pbGate.GateClient
	locator        *locator.GateLocator
	registry       registry.Registry
	errorHandler   ErrorHandler
	rpcServer      *rpc.Server
	rpcListener    net.Listener
	ctx            context.Context
	cancel         context.CancelFunc
	locatorTimeout time.Duration
	started        atomic.Bool
	closed         atomic.Bool
	serverWait     sync.WaitGroup
	wait           sync.WaitGroup
}

func New(config Config) *Gate {
	if config.Instance.Id == "" || config.Instance.ServiceName != locator.GateName {
		panic("gate: 必须提供Gate ServiceInstance")
	}
	if config.Router == nil {
		panic("gate: 必须提供Router")
	}
	if config.NodeClient == nil {
		panic("gate: 必须提供NodeClient")
	}
	if config.GateClient == nil {
		panic("gate: 必须提供GateClient")
	}
	if config.Locator == nil {
		panic("gate: 必须提供GateLocator")
	}
	if config.Registry == nil {
		panic("gate: 必须提供Registry")
	}
	if config.RPCServer == nil {
		panic("gate: 必须提供TCP RPC Server")
	}
	if config.RPCListener == nil {
		panic("gate: 必须提供TCP RPC Listener")
	}
	if config.LocatorTimeout <= 0 {
		config.LocatorTimeout = defaultLocatorTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := &Gate{
		App:            app.New(config.Components...),
		instance:       config.Instance,
		router:         config.Router,
		nodeClient:     config.NodeClient,
		gateClient:     config.GateClient,
		locator:        config.Locator,
		registry:       config.Registry,
		errorHandler:   config.ErrorHandler,
		rpcServer:      config.RPCServer,
		rpcListener:    config.RPCListener,
		ctx:            ctx,
		cancel:         cancel,
		locatorTimeout: config.LocatorTimeout,
	}
	if g.errorHandler == nil {
		g.errorHandler = defaultErrorHandler
	}

	options := append([]network.Option(nil), config.NetworkOptions...)
	options = append(options, network.WithHooks(network.Hooks{
		OnSessionStart: config.Hooks.OnSessionStart,
		OnSessionBind: func(session *network.Session) error {
			if err := g.onSessionBind(session); err != nil {
				return err
			}
			if config.Hooks.OnSessionBind != nil {
				return config.Hooks.OnSessionBind(session)
			}
			return nil
		},
		OnSessionEnd: func(ctx context.Context, session *network.Session) {
			g.onSessionEnd(ctx, session)
			if config.Hooks.OnSessionEnd != nil {
				help.SafeRun(func() {
					config.Hooks.OnSessionEnd(ctx, session)
				})
			}
		},
		OnHeartbeat: config.Hooks.OnHeartbeat,
		OnReq: func(ctx *network.ReqContext) {
			g.onReq(ctx)
			if config.Hooks.OnReq != nil {
				help.SafeRun(func() {
					config.Hooks.OnReq(ctx)
				})
			}
		},
	}))
	g.server = network.NewServer(options...)
	g.locator.SetOnBindingLost(func(uid uint64, binding locator.GateBinding) {
		g.server.KickUidSession(uid, binding.SessionId)
	})

	pbGate.RegisterGateServer(config.RPCServer, g)
	return g
}

func (g *Gate) Start() error {
	if g.closed.Load() {
		return ErrClosed
	}
	if !g.started.CompareAndSwap(false, true) {
		return ErrStarted
	}
	if err := g.App.Start(); err != nil {
		g.cancel()
		g.closed.Store(true)
		return err
	}
	g.serverWait.Add(1)
	help.SafeGo(func() {
		defer g.serverWait.Done()
		if err := g.rpcServer.Serve(g.rpcListener); err != nil && !g.closed.Load() {
			logx.Errorf("gate: TCP RPC服务退出: %v", err)
		}
	})
	if err := g.registry.Register(g.instance); err != nil {
		g.stopAfterStartFailure()
		return err
	}
	if err := g.server.Start(); err != nil {
		_ = g.registry.Deregister(g.instance.Id)
		g.stopAfterStartFailure()
		return err
	}
	return nil
}

func (g *Gate) stopAfterStartFailure() {
	g.cancel()
	_ = g.server.Shutdown(context.Background())
	_ = g.rpcServer.Shutdown(context.Background())
	g.serverWait.Wait()
	_ = g.App.Shutdown(context.Background())
	g.Wait()
	g.closed.Store(true)
}

func (g *Gate) Shutdown(ctx context.Context) error {
	if !g.closed.CompareAndSwap(false, true) {
		return nil
	}

	if g.started.Load() {
		_ = g.registry.Deregister(g.instance.Id)
	}
	g.App.RequestStop()
	serverErr := g.server.Shutdown(ctx)

	_ = g.rpcServer.Shutdown(ctx)
	g.serverWait.Wait()
	g.cancel()

	componentErr := g.App.Shutdown(ctx)

	waitDone := make(chan struct{})
	help.SafeGo(func() {
		defer close(waitDone)
		g.Wait()
	})
	select {
	case <-waitDone:
	case <-ctx.Done():
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if serverErr != nil {
		return serverErr
	}
	return componentErr
}

func (g *Gate) AddWait() {
	g.wait.Add(1)
}

func (g *Gate) DoneWait() {
	g.wait.Done()
}

func (g *Gate) Wait() {
	g.wait.Wait()
}

func (g *Gate) UpdateMetadata(metadata map[string]string) error {
	err := g.registry.UpdateMetaData(g.instance.Id, metadata)
	if err != nil {
		return err
	}
	if g.instance.MetaData == nil {
		g.instance.MetaData = make(map[string]string)
	}
	for key, value := range metadata {
		g.instance.MetaData[key] = value
	}
	return nil
}

func (g *Gate) DeleteMetadata(keys ...string) error {
	err := g.registry.DeleteMetaData(g.instance.Id, keys)
	if err != nil {
		return err
	}
	for _, key := range keys {
		delete(g.instance.MetaData, key)
	}
	return nil
}

func (g *Gate) Done() <-chan struct{} {
	return g.ctx.Done()
}

func (g *Gate) onReq(request *network.ReqContext) {
	message := request.Request
	ctx := &Context{
		Context:    request.Context,
		App:        g,
		Session:    request.Session,
		Uid:        request.Session.Uid(),
		Route:      message.Route,
		Seq:        message.Seq,
		Body:       message.Body,
		BindingKey: strconv.FormatUint(request.Session.Uid(), 10),
		needReply:  request.NeedReply,
		forward:    g.forward,
	}

	if err := g.router.Dispatch(ctx); err != nil {
		ctx.replied, ctx.responseBody = false, nil
		help.SafeRun(func() {
			g.errorHandler(ctx, err)
		})
	}
	if ctx.replied {
		if err := request.Write(ctx.responseBody); err != nil {
			logx.Errorf("gate: 写入响应失败: %v", err)
		}
	}
}

func (g *Gate) forward(ctx *Context) error {
	target := ctx.Target
	if err := validateTarget(&target); err != nil {
		return err
	}
	ctx.Target = target

	rpcCtx := ctx.Context
	switch target.Mode {
	case RouteModeBalance:
		rpcCtx = lx.WithBalance(rpcCtx, target.Service, target.Balance)
	case RouteModeSelect:
		rpcCtx = lx.WithSelect(rpcCtx, target.Service, target.Binding, ctx.BindingKey)
	case RouteModeActor:
		ctx.ActorKey = ctx.actorKeyResolver(ctx)
		if ctx.ActorKey == "" {
			return ErrInvalidTarget
		}
		rpcCtx = lx.WithActor(rpcCtx, target.Service, ctx.ActorKey)
	case RouteModeNode:
		rpcCtx = lx.WithNode(rpcCtx, target.NodeId)
	}

	request := &pbNode.Request{
		Uid:             ctx.Uid,
		Route:           ctx.Route,
		Body:            ctx.Body,
		GateServiceName: g.instance.ServiceName,
		GateInstanceId:  g.instance.Id,
		ActorKey:        ctx.ActorKey,
	}
	if !ctx.NeedReply() {
		_, err := g.nodeClient.Tell(rpcCtx, request)
		var redirect *rpc.Error
		if errors.As(err, &redirect) && redirect.Code == rpc.ErrorCodeRedirect {
			_, err = g.nodeClient.Tell(lx.WithNode(ctx.Context, string(redirect.Detail)), request)
		}
		return err
	}

	response, err := g.nodeClient.Call(rpcCtx, request)
	var redirect *rpc.Error
	if errors.As(err, &redirect) && redirect.Code == rpc.ErrorCodeRedirect {
		response, err = g.nodeClient.Call(lx.WithNode(ctx.Context, string(redirect.Detail)), request)
	}
	if err != nil {
		return err
	}
	if response.NodeServiceName == "" || response.NodeInstanceId == "" {
		return ErrInvalidNodeSource
	}
	ctx.NodeServiceName = response.NodeServiceName
	ctx.NodeInstanceId = response.NodeInstanceId
	if response.Replied {
		return ctx.Reply(response.Body)
	}
	return nil
}

func (g *Gate) onSessionBind(session *network.Session) error {
	uid := session.Uid()
	current := locator.GateBinding{
		InstanceId: g.instance.Id,
		SessionId:  session.Id,
	}
	ctx, cancel := context.WithTimeout(session.Context(), g.locatorTimeout)
	defer cancel()
	previous, err := g.locator.LocateBinding(ctx, uid)
	if err != nil {
		return err
	}
	// Do not overwrite an unreachable Gate's record and restart its TTL on
	// every reconnect. Let its original lease expire before taking over.
	if previous.InstanceId != "" && previous.InstanceId != current.InstanceId {
		if _, err := g.gateClient.Kick(lx.WithNode(ctx, previous.InstanceId), &pbGate.KickRequest{
			Uid: uid, NodeServiceName: g.instance.ServiceName, NodeInstanceId: g.instance.Id, SessionId: previous.SessionId,
		}); err != nil {
			return err
		}
	}
	return g.locator.Bind(ctx, uid, current)
}

func (g *Gate) onSessionEnd(parent context.Context, session *network.Session) {
	uid := session.Uid()
	if uid == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(parent, g.locatorTimeout)
	err := g.locator.Unbind(ctx, uid, locator.GateBinding{
		InstanceId: g.instance.Id,
		SessionId:  session.Id,
	})
	cancel()
	if err != nil {
		logx.Errorf("gate: 解绑UID定位失败 uid=%d instance=%s err=%v", uid, g.instance.Id, err)
	}
}

func defaultErrorHandler(ctx *Context, err error) {
	logx.Errorf(
		"gate: 请求处理失败 uid=%d route=%d routeID=%s targetService=%s targetNode=%s err=%v",
		ctx.Uid,
		ctx.Route,
		ctx.RouteId,
		ctx.Target.Service,
		ctx.Target.NodeId,
		err,
	)
}
