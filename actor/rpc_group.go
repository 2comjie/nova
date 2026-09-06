package actor

import (
	"context"
	"slices"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/logx"
)

type RPCHandler[T actorDef.Actor] func(actorValue T, pid actorDef.Pid, ctx context.Context, message Message) ([]byte, error)

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
		for _, middleware := range slices.Backward(middlewares) {
			handler = middleware(handler)
		}
	}

	processor := rpcProcessor(func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message, needReply bool) ([]byte, bool, error) {
		runner, handled, err := g.actors.ResolveActor(ctx, key, policy)
		if err != nil || !handled {
			return nil, handled, err
		}

		if !needReply {
			err := runner.RunOnMainLoop(func(actorValue T) {
				if _, err := handler(actorValue, runner.self, runner.runCtx, message); err != nil {
					logx.Errorf("actor %s route %d failed: %v", runner.self, message.Route, err)
				}
			})
			return nil, err == nil, err
		}

		var body []byte
		err = runner.WaitResultOnMainLoop(ctx, func(execCtx context.Context, actorValue T) error {
			var err error
			body, err = handler(actorValue, runner.self, execCtx, message)
			return err
		})
		if err != nil {
			return nil, false, err
		}
		return body, true, nil
	})

	g.actors.system.registrations[g.actors.actorType].routes[route] = processor
}
