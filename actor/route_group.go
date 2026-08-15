package actor

import (
	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/app/node"
)

type KeyResolver func(ctx *node.Context) actorDef.Key

type RouteGroup[T actorDef.Actor] struct {
	router     *node.RouteGroup
	parent     *RouteGroup[T]
	server     *Server
	system     *System[T]
	policy     ActivationPolicy
	resolveKey KeyResolver
	middleware []Middleware[T]
}

func NewRouteGroup[T actorDef.Actor](router *node.Router, server *Server, system *System[T], policy ActivationPolicy, resolveKey KeyResolver) *RouteGroup[T] {
	return &RouteGroup[T]{router: router.Group(), server: server, system: system, policy: policy, resolveKey: resolveKey}
}

func (g *RouteGroup[T]) Use(middlewares ...Middleware[T]) {
	g.middleware = append(g.middleware, middlewares...)
}

func (g *RouteGroup[T]) Group() *RouteGroup[T] {
	return &RouteGroup[T]{router: g.router.Group(), parent: g, server: g.server, system: g.system, policy: g.policy, resolveKey: g.resolveKey}
}

func (g *RouteGroup[T]) Handle(route uint32, handler Handler[T]) {
	groups := make([]*RouteGroup[T], 0, 2)
	for current := g; current != nil; current = current.parent {
		groups = append(groups, current)
	}
	for groupIndex := 0; groupIndex < len(groups); groupIndex++ {
		middlewares := groups[groupIndex].middleware
		for middlewareIndex := len(middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
			handler = middlewares[middlewareIndex](handler)
		}
	}

	if err := g.router.Handle(route, func(ctx *node.Context) error {
		return g.server.Handle(ctx, g.resolveKey(ctx), g.policy)
	}); err != nil {
		panic(err)
	}
	Reg(g.server, g.system, route, handler)
}
