package actor

import (
	"context"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/core/help"
)

type RPCHandler[T actorDef.Actor] func(actorValue T, pid actorDef.PID, ctx context.Context, message Message) ([]byte, error)

type RPCMiddleware[T actorDef.Actor] func(next RPCHandler[T]) RPCHandler[T]

type RPCRouteGroup[T actorDef.Actor] struct {
	parent     *RPCRouteGroup[T]
	actors     *Manager[T]
	middleware []RPCMiddleware[T]
}

func (g *RPCRouteGroup[T]) Use(middlewares ...RPCMiddleware[T]) {
	g.middleware = append(g.middleware, middlewares...)
}

func (g *RPCRouteGroup[T]) Group() *RPCRouteGroup[T] {
	return &RPCRouteGroup[T]{parent: g, actors: g.actors}
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
		runner, handled, err := g.actors.ResolveActor(ctx, key, policy)
		if err != nil || !handled {
			return nil, handled, err
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
		if !needReply {
			body = nil
		}
		return body, true, handleErr
	})

	g.actors.system.registerRoute(g.actors.actorType, route, processor)
}
