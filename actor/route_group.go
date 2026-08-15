package actor

import (
	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/app/node"
)

type KeyResolver func(ctx *node.Context) actorDef.Key

type RouteGroup[T actorDef.Actor] struct {
	router     *node.Router
	server     *Server
	system     *System[T]
	policy     ActivationPolicy
	resolveKey KeyResolver
}

func NewRouteGroup[T actorDef.Actor](router *node.Router, server *Server, system *System[T], policy ActivationPolicy, resolveKey KeyResolver) *RouteGroup[T] {
	return &RouteGroup[T]{router: router, server: server, system: system, policy: policy, resolveKey: resolveKey}
}

func (g *RouteGroup[T]) RegAsk(route uint32, handler Handler[T]) error {
	if err := RegAsk(g.server, g.system, route, handler); err != nil {
		return err
	}
	if err := g.router.Handle(route, func(ctx *node.Context) error {
		return g.server.Handle(ctx, g.resolveKey(ctx), g.policy)
	}); err != nil {
		g.server.mu.Lock()
		delete(g.server.askRoutes, route)
		g.server.mu.Unlock()
		return err
	}
	return nil
}

func (g *RouteGroup[T]) RegTell(route uint32, handler Handler[T]) error {
	if err := RegTell(g.server, g.system, route, handler); err != nil {
		return err
	}
	if err := g.router.Handle(route, func(ctx *node.Context) error {
		return g.server.Handle(ctx, g.resolveKey(ctx), g.policy)
	}); err != nil {
		g.server.mu.Lock()
		delete(g.server.tellRoutes, route)
		g.server.mu.Unlock()
		return err
	}
	return nil
}
