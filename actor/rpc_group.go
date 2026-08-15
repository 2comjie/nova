package actor

import (
	"bytes"
	"context"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
)

type RPCHandler[T actorDef.Actor] func(actorValue T, pid actorDef.PID, ctx context.Context, message Message) ([]byte, error)

type RPCMiddleware[T actorDef.Actor] func(next RPCHandler[T]) RPCHandler[T]

type RPCRouteGroup[T actorDef.Actor] struct {
	server     *Server
	parent     *RPCRouteGroup[T]
	system     *System[T]
	middleware []RPCMiddleware[T]
}

func NewRPCRouteGroup[T actorDef.Actor](server *Server, system *System[T]) *RPCRouteGroup[T] {
	server.mu.Lock()
	server.stops[system.actorType] = system.Stop
	server.mu.Unlock()
	return &RPCRouteGroup[T]{server: server, system: system}
}

func (g *RPCRouteGroup[T]) Use(middlewares ...RPCMiddleware[T]) {
	g.middleware = append(g.middleware, middlewares...)
}

func (g *RPCRouteGroup[T]) Group() *RPCRouteGroup[T] {
	return &RPCRouteGroup[T]{server: g.server, parent: g, system: g.system}
}

func (g *RPCRouteGroup[T]) Handle(route uint32, handler RPCHandler[T]) {
	groups := make([]*RPCRouteGroup[T], 0, 2)
	for current := g; current != nil; current = current.parent {
		groups = append(groups, current)
	}
	for groupIndex := 0; groupIndex < len(groups); groupIndex++ {
		middlewares := groups[groupIndex].middleware
		for middlewareIndex := len(middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
			handler = middlewares[middlewareIndex](handler)
		}
	}

	processor := rpcProcessor(func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message, needReply bool) ([]byte, bool, error) {
		runner, handled, err := g.system.ResolveActor(ctx, key, policy)
		if err != nil || !handled {
			return nil, handled, err
		}

		if !needReply {
			message.Body = bytes.Clone(message.Body)
			err = runner.RunOnMainLoop(func(actorValue T) {
				var handleErr error = ErrMessageHandlerPanic
				help.SafeRun(func() {
					_, handleErr = handler(actorValue, runner.self, context.WithoutCancel(runner.runCtx), message)
				})
				if handleErr != nil {
					logx.Errorf("actor: RPC Tell处理失败 route=%d pid=%s err=%v", message.Route, runner.self.String(), handleErr)
				}
			})
			return nil, err == nil, err
		}

		var body []byte
		var handleErr error = ErrMessageHandlerPanic
		err = runner.WaitResultOnMainLoop(ctx, func(actorValue T) {
			help.SafeRun(func() {
				body, handleErr = handler(actorValue, runner.self, ctx, message)
			})
		})
		if err != nil {
			return nil, false, err
		}
		return body, true, handleErr
	})

	g.server.mu.Lock()
	routes := g.server.routes[g.system.actorType]
	if routes == nil {
		routes = make(map[uint32]rpcProcessor)
		g.server.routes[g.system.actorType] = routes
	}
	if routes[route] == nil {
		routes[route] = processor
	}
	g.server.mu.Unlock()
}
