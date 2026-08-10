package gate

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network"
	"github.com/2comjie/wali/registry"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
)

const defaultLocatorTimeout = 3 * time.Second

var (
	ErrInstanceRequired    = errors.New("gate: 必须提供Gate ServiceInstance")
	ErrRouterRequired      = errors.New("gate: 必须提供Router")
	ErrNodeClientRequired  = errors.New("gate: 必须提供NodeClient")
	ErrGateClientRequired  = errors.New("gate: 必须提供GateClient")
	ErrLocatorRequired     = errors.New("gate: 必须提供GateLocator")
	ErrRegistryRequired    = errors.New("gate: 必须提供Registry")
	ErrRPCServerRequired   = errors.New("gate: 必须提供gRPC Server")
	ErrRPCListenerRequired = errors.New("gate: 必须提供gRPC Listener")
	ErrStarted             = errors.New("gate: Gate已经启动")
	ErrClosed              = errors.New("gate: Gate已经关闭")
	ErrHandlerPanic        = errors.New("gate: Filter或Forward发生panic")
	ErrInvalidNodeSource   = errors.New("gate: Node来源信息无效")
)

type ErrorHandler func(ctx *Context, err error)

type Config struct {
	Instance       endpoint.ServiceInstance
	Router         *Router
	NodeClient     pbNode.NodeClient
	GateClient     pbGate.GateClient
	Locator        *locator.GateLocator
	Registry       registry.Registry
	RPCServer      *grpc.Server
	RPCListener    net.Listener
	NetworkOptions []network.Option
	Hooks          network.Hooks
	ErrorHandler   ErrorHandler
	LocatorTimeout time.Duration
}

type Gate struct {
	pbGate.UnimplementedGateServer

	instance     endpoint.ServiceInstance
	router       *Router
	server       *network.Server
	nodeClient   pbNode.NodeClient
	gateClient   pbGate.GateClient
	locator      *locator.GateLocator
	registry     registry.Registry
	errorHandler ErrorHandler
	rpcServer    *grpc.Server
	rpcListener  net.Listener

	ctx            context.Context
	cancel         context.CancelFunc
	locatorTimeout time.Duration
	sessions       sync.Map
	started        atomic.Bool
	closed         atomic.Bool
	serverWait     sync.WaitGroup
	wait           sync.WaitGroup
}

func New(config Config) (*Gate, error) {
	if config.Instance.ID == "" || config.Instance.ServiceName != locator.GateName {
		return nil, ErrInstanceRequired
	}
	if config.Router == nil {
		return nil, ErrRouterRequired
	}
	if config.NodeClient == nil {
		return nil, ErrNodeClientRequired
	}
	if config.GateClient == nil {
		return nil, ErrGateClientRequired
	}
	if config.Locator == nil {
		return nil, ErrLocatorRequired
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
	if config.LocatorTimeout <= 0 {
		config.LocatorTimeout = defaultLocatorTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := &Gate{
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
		OnSessionEnd: func(session *network.Session) {
			g.onSessionEnd(session)
			if config.Hooks.OnSessionEnd != nil {
				help.SafeRun(func() {
					config.Hooks.OnSessionEnd(session)
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
	server, err := network.NewServer(options...)
	if err != nil {
		cancel()
		return nil, err
	}
	g.server = server

	if err := g.router.Freeze(); err != nil {
		cancel()
		_ = server.Shutdown(context.Background())
		return nil, err
	}
	pbGate.RegisterGateServer(config.RPCServer, g)
	return g, nil
}

func (g *Gate) Start() error {
	if g.closed.Load() {
		return ErrClosed
	}
	if !g.started.CompareAndSwap(false, true) {
		return ErrStarted
	}
	g.serverWait.Add(1)
	help.SafeGo(func() {
		defer g.serverWait.Done()
		if err := g.rpcServer.Serve(g.rpcListener); err != nil && !g.closed.Load() {
			logx.Errorf("gate: gRPC服务退出: %v", err)
		}
	})
	if err := g.registry.Register(g.instance); err != nil {
		g.cancel()
		_ = g.server.Shutdown(context.Background())
		g.rpcServer.Stop()
		g.serverWait.Wait()
		g.Wait()
		g.closed.Store(true)
		return err
	}
	if err := g.server.Start(); err != nil {
		_ = g.registry.Deregister(g.instance.ID)
		g.cancel()
		_ = g.server.Shutdown(context.Background())
		g.rpcServer.Stop()
		g.serverWait.Wait()
		g.Wait()
		g.closed.Store(true)
		return err
	}
	return nil
}

func (g *Gate) Shutdown(ctx context.Context) error {
	if !g.closed.CompareAndSwap(false, true) {
		return nil
	}

	var errs []error
	if g.started.Load() {
		if err := g.registry.Deregister(g.instance.ID); err != nil {
			errs = append(errs, err)
		}
	}
	if err := g.server.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}

	rpcDone := make(chan struct{})
	help.SafeGo(func() {
		defer close(rpcDone)
		g.rpcServer.GracefulStop()
	})
	select {
	case <-rpcDone:
	case <-ctx.Done():
		g.rpcServer.Stop()
		<-rpcDone
		errs = append(errs, ctx.Err())
	}
	g.serverWait.Wait()
	g.cancel()

	waitDone := make(chan struct{})
	help.SafeGo(func() {
		defer close(waitDone)
		g.Wait()
	})
	select {
	case <-waitDone:
	case <-ctx.Done():
		errs = append(errs, ctx.Err())
	}
	return errors.Join(errs...)
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
	err := g.registry.UpdateMetaData(g.instance.ID, metadata)
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
	err := g.registry.DeleteMetaData(g.instance.ID, keys)
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
		Context:    g.ctx,
		App:        g,
		Session:    request.Session,
		Route:      message.Route,
		Seq:        message.Seq,
		Body:       message.Body,
		BindingKey: request.Session.UID(),
		needReply:  request.NeedReply,
		forward:    g.forward,
	}

	var err error
	completed := false
	help.SafeRun(func() {
		err = g.router.Dispatch(ctx)
		completed = true
	})
	if !completed {
		err = ErrHandlerPanic
	}
	if err == nil && request.NeedReply && ctx.replied {
		err = request.Write(ctx.responseBody)
	}
	if err != nil {
		help.SafeRun(func() {
			g.errorHandler(ctx, err)
		})
	}
}

func (g *Gate) forward(ctx *Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	case RouteModeNode:
		rpcCtx = lx.WithNode(rpcCtx, target.NodeID)
	}

	request := &pbNode.Request{
		Uid:             ctx.Session.UID(),
		Route:           ctx.Route,
		Body:            ctx.Body,
		GateServiceName: g.instance.ServiceName,
		GateInstanceId:  g.instance.ID,
	}
	if !ctx.NeedReply() {
		_, err := g.nodeClient.Tell(rpcCtx, request)
		return err
	}

	response, err := g.nodeClient.Call(rpcCtx, request)
	if err != nil {
		return err
	}
	if response.NodeServiceName == "" || response.NodeInstanceId == "" {
		return ErrInvalidNodeSource
	}
	ctx.NodeServiceName = response.NodeServiceName
	ctx.NodeInstanceID = response.NodeInstanceId
	if response.Replied {
		return ctx.Reply(response.Body)
	}
	return nil
}

func (g *Gate) onSessionBind(session *network.Session) error {
	uid := session.UID()
	if uid == "" {
		return network.ErrUnauthorized
	}
	g.sessions.Store(uid, session.ID)

	current := locator.GateBinding{
		InstanceID: g.instance.ID,
		SessionID:  session.ID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), g.locatorTimeout)
	previous, err := g.locator.Bind(ctx, uid, current)
	cancel()
	if err != nil {
		logx.Errorf("gate: 绑定UID定位失败 uid=%s instance=%s err=%v", uid, g.instance.ID, err)
		return err
	}
	if previous.InstanceID == "" || previous.InstanceID == current.InstanceID {
		return nil
	}

	ctx, cancel = context.WithTimeout(context.Background(), g.locatorTimeout)
	_, kickErr := g.gateClient.Kick(lx.WithNode(ctx, previous.InstanceID), &pbGate.KickRequest{
		Uid:             uid,
		NodeServiceName: g.instance.ServiceName,
		NodeInstanceId:  g.instance.ID,
		SessionId:       previous.SessionID,
	})
	cancel()
	if kickErr == nil {
		return nil
	}
	logx.Errorf("gate: 踢出旧Gate Session失败 uid=%s instance=%s session=%d err=%v", uid, previous.InstanceID, previous.SessionID, kickErr)

	ctx, cancel = context.WithTimeout(context.Background(), g.locatorTimeout)
	restored, restoreErr := g.locator.Restore(ctx, uid, current, previous)
	cancel()
	if restoreErr != nil {
		logx.Errorf("gate: 恢复UID旧定位失败 uid=%s current=%s previous=%s err=%v", uid, current.InstanceID, previous.InstanceID, restoreErr)
		return errors.Join(kickErr, restoreErr)
	}
	if !restored {
		logx.Warnf("gate: UID定位已被更新，忽略旧定位恢复 uid=%s current=%s previous=%s", uid, current.InstanceID, previous.InstanceID)
	}
	return kickErr
}

func (g *Gate) onSessionEnd(session *network.Session) {
	uid := session.UID()
	if uid == "" || !g.sessions.CompareAndDelete(uid, session.ID) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.locatorTimeout)
	err := g.locator.Unbind(ctx, uid, locator.GateBinding{
		InstanceID: g.instance.ID,
		SessionID:  session.ID,
	})
	cancel()
	if err != nil {
		logx.Errorf("gate: 解绑UID定位失败 uid=%s instance=%s err=%v", uid, g.instance.ID, err)
	}
}

func defaultErrorHandler(ctx *Context, err error) {
	logx.Errorf(
		"gate: 请求处理失败 uid=%s route=%d routeID=%s targetService=%s targetNode=%s err=%v",
		ctx.Session.UID(),
		ctx.Route,
		ctx.RouteID,
		ctx.Target.Service,
		ctx.Target.NodeID,
		err,
	)
}
