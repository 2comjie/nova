package actor

import (
	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/app/node"
)

type KeyResolver func(ctx *node.Context) actorDef.Key

func Middleware(server *Server, policy ActivationPolicy, resolveKey KeyResolver) node.Middleware {
	return func(next node.Handler) node.Handler {
		return func(ctx *node.Context) error {
			if !server.HasRoute(ctx.Request.Route) {
				return next(ctx)
			}
			return server.Handle(ctx, resolveKey(ctx), policy)
		}
	}
}
