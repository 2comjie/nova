package actor

import (
	"slices"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/app/node"
)

type Handler[T actorDef.Actor] func(actorValue T, pid actorDef.Pid, ctx *node.Context) error

type Middleware[T actorDef.Actor] func(next Handler[T]) Handler[T]

type RouteGroup[T actorDef.Actor] struct {
	router     *node.RouteGroup
	parent     *RouteGroup[T]
	actors     *Manager[T]
	policy     ActivationPolicy
	middleware []Middleware[T]
}

func NewRouteGroup[T actorDef.Actor](router *node.Router, actors *Manager[T], policy ActivationPolicy) *RouteGroup[T] {
	return &RouteGroup[T]{router: router.Group(), actors: actors, policy: policy}
}

func (g *RouteGroup[T]) Use(middlewares ...Middleware[T]) {
	g.middleware = append(g.middleware, middlewares...)
}

func (g *RouteGroup[T]) Group() *RouteGroup[T] {
	return &RouteGroup[T]{router: g.router.Group(), parent: g, actors: g.actors, policy: g.policy}
}

func (g *RouteGroup[T]) Handle(route uint32, handler Handler[T]) {
	groups := make([]*RouteGroup[T], 0, 2)
	for current := g; current != nil; current = current.parent {
		groups = append(groups, current)
	}
	for groupIndex := 0; groupIndex < len(groups); groupIndex++ {
		middlewares := groups[groupIndex].middleware
		for _, middleware := range slices.Backward(middlewares) {
			handler = middleware(handler)
		}
	}

	g.router.Handle(route, func(ctx *node.Context) error {
		runner, handled, err := g.actors.ResolveActor(ctx, actorDef.Key(ctx.Request.ActorKey), g.policy)
		if err != nil || !handled {
			return err
		}

		var handleErr error = ErrMessageHandlerPanic
		err = runner.WaitResultOnMainLoop(ctx, func(actorValue T) {
			handleErr = handler(actorValue, runner.self, ctx)
		})
		if err != nil {
			return err
		}
		return handleErr
	})
}
