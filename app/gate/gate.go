package gate

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	"google.golang.org/grpc"
)

const defaultLocatorTimeout = 3 * time.Second

var (
	ErrStarted           = errors.New("gate: Gate已经启动")
	ErrClosed            = errors.New("gate: Gate已经关闭")
	ErrHandlerPanic      = errors.New("gate: Filter或Forward发生panic")
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
	RPCServer      *grpc.Server
	RPCListener    net.Listener
	NetworkOptions []network.Option
	Hooks          network.Hooks
	ErrorHandler   ErrorHandler
	LocatorTimeout time.Duration
	Components     []app.Component
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
	components   []app.Component
	componentMu  sync.Mutex

	ctx               context.Context
	cancel            context.CancelFunc
	locatorTimeout    time.Duration
	sessions          sync.Map
	started           atomic.Bool
	closed            atomic.Bool
	startedComponents atomic.Int64
	serverWait        sync.WaitGroup
	wait              sync.WaitGroup
}

func New(config Config) *Gate {
	if config.Instance.ID == "" || config.Instance.ServiceName != locator.GateName {
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
		panic("gate: 必须提供gRPC Server")
	}
	if config.RPCListener == nil {
		panic("gate: 必须提供gRPC Listener")
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
		components:     append([]app.Component(nil), config.Components...),
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
		panic(err)
	}
	g.server = server

	if err := g.router.Freeze(); err != nil {
		panic(err)
	}
	pbGate.RegisterGateServer(config.RPCServer, g)
	return g
}

func (g *Gate) AddComponent(component app.Component) error {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	if g.closed.Load() {
		return ErrClosed
	}
	if g.started.Load() {
		return ErrStarted
	}
	g.components = append(g.components, component)
	return nil
}

func (g *Gate) Start() error {
	g.componentMu.Lock()
	if g.closed.Load() {
		g.componentMu.Unlock()
		return ErrClosed
	}
	if !g.started.CompareAndSwap(false, true) {
		g.componentMu.Unlock()
		return ErrStarted
	}
	g.componentMu.Unlock()

	for _, component := range g.components {
		logx.Infof("gate: 正在启动组件 name=%s", component.Name())
		if err := component.Start(); err != nil {
			var errs []error
			errs = append(errs, fmt.Errorf("gate: 启动组件失败 name=%s: %w", component.Name(), err))
			g.cancel()
			_ = g.server.Shutdown(context.Background())
			g.rpcServer.Stop()
			for index := int(g.startedComponents.Load()) - 1; index >= 0; index-- {
				startedComponent := g.components[index]
				logx.Infof("gate: 正在回滚组件 name=%s", startedComponent.Name())
				if shutdownErr := startedComponent.Shutdown(context.Background()); shutdownErr != nil {
					errs = append(errs, fmt.Errorf("gate: 回滚组件失败 name=%s: %w", startedComponent.Name(), shutdownErr))
				}
			}
			g.Wait()
			g.closed.Store(true)
			return errors.Join(errs...)
		}
		g.startedComponents.Add(1)
		logx.Infof("gate: 组件启动完成 name=%s", component.Name())
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
		for index := int(g.startedComponents.Load()) - 1; index >= 0; index-- {
			component := g.components[index]
			logx.Infof("gate: 正在回滚组件 name=%s", component.Name())
			if shutdownErr := component.Shutdown(context.Background()); shutdownErr != nil {
				err = errors.Join(err, fmt.Errorf("gate: 回滚组件失败 name=%s: %w", component.Name(), shutdownErr))
			}
		}
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
		for index := int(g.startedComponents.Load()) - 1; index >= 0; index-- {
			component := g.components[index]
			logx.Infof("gate: 正在回滚组件 name=%s", component.Name())
			if shutdownErr := component.Shutdown(context.Background()); shutdownErr != nil {
				err = errors.Join(err, fmt.Errorf("gate: 回滚组件失败 name=%s: %w", component.Name(), shutdownErr))
			}
		}
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

	for index := int(g.startedComponents.Load()) - 1; index >= 0; index-- {
		component := g.components[index]
		logx.Infof("gate: 正在关闭组件 name=%s", component.Name())
		if err := component.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("gate: 关闭组件失败 name=%s: %w", component.Name(), err))
			continue
		}
		logx.Infof("gate: 组件关闭完成 name=%s", component.Name())
	}

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
		Uid:        request.Session.UID(),
		Route:      message.Route,
		Seq:        message.Seq,
		Body:       message.Body,
		BindingKey: request.Session.UID(),
		needReply:  request.NeedReply,
		forward:    g.forward,
	}

	err := g.dispatch(ctx)
	if err == nil && request.NeedReply && ctx.replied {
		err = request.Write(ctx.responseBody)
	}
	if err != nil {
		help.SafeRun(func() {
			g.errorHandler(ctx, err)
		})
	}
}

func (g *Gate) dispatch(ctx *Context) error {
	var err error
	completed := false
	help.SafeRun(func() {
		err = g.router.Dispatch(ctx)
		completed = true
	})
	if !completed {
		return ErrHandlerPanic
	}
	return err
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
	case RouteModeActor:
		ctx.ActorKey = ctx.actorKeyResolver(ctx)
		if ctx.ActorKey == "" {
			return ErrInvalidTarget
		}
		rpcCtx = lx.WithActor(rpcCtx, target.Service, ctx.ActorKey)
	case RouteModeNode:
		rpcCtx = lx.WithNode(rpcCtx, target.NodeID)
	}

	request := &pbNode.Request{
		Uid:             ctx.Uid,
		Route:           ctx.Route,
		Body:            ctx.Body,
		GateServiceName: g.instance.ServiceName,
		GateInstanceId:  g.instance.ID,
		ActorKey:        ctx.ActorKey,
	}
	if !ctx.NeedReply() {
		_, err := g.nodeClient.Tell(rpcCtx, request)
		if err != nil && err.Code() == rpc.ErrorCodeRedirect {
			_, err = g.nodeClient.Tell(lx.WithNode(ctx.Context, string(err.Detail())), request)
		}
		return err
	}

	response, err := g.nodeClient.Call(rpcCtx, request)
	if err != nil && err.Code() == rpc.ErrorCodeRedirect {
		response, err = g.nodeClient.Call(lx.WithNode(ctx.Context, string(err.Detail())), request)
	}
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
