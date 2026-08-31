package node

import "errors"

var (
	ErrInvalidRoute  = errors.New("node: route必须大于0")
	ErrNilHandler    = errors.New("node: Handler不能为空")
	ErrRouteNotFound = errors.New("node: route不存在")
)

type Handler func(*Context) error

type Middleware func(next Handler) Handler

type Router struct {
	routes     map[uint32]Handler
	middleware []Middleware
}

func NewRouter() *Router {
	return &Router{routes: make(map[uint32]Handler)}
}

func (r *Router) Use(middlewares ...Middleware) {
	r.middleware = append(r.middleware, middlewares...)
}

func (r *Router) Handle(route uint32, handler Handler) {
	r.add(route, handler, nil)
}

func (r *Router) Group() *RouteGroup {
	return &RouteGroup{router: r}
}

func (r *Router) Dispatch(ctx *Context) error {
	handler := r.routes[ctx.Request.Route]
	if handler == nil {
		return ErrRouteNotFound
	}
	return handler(ctx)
}

func (r *Router) add(route uint32, handler Handler, group *RouteGroup) {
	if route == 0 {
		panic(ErrInvalidRoute)
	}
	if handler == nil {
		panic(ErrNilHandler)
	}

	middlewares := append([]Middleware(nil), r.middleware...)
	if group != nil {
		var groups []*RouteGroup
		for current := group; current != nil; current = current.parent {
			groups = append(groups, current)
		}
		for index := len(groups) - 1; index >= 0; index-- {
			middlewares = append(middlewares, groups[index].middlewares...)
		}
	}
	for index := len(middlewares) - 1; index >= 0; index-- {
		handler = middlewares[index](handler)
	}
	r.routes[route] = handler
}

type RouteGroup struct {
	router      *Router
	parent      *RouteGroup
	middlewares []Middleware
}

func (g *RouteGroup) Group() *RouteGroup {
	return &RouteGroup{router: g.router, parent: g}
}

func (g *RouteGroup) Use(middlewares ...Middleware) {
	g.middlewares = append(g.middlewares, middlewares...)
}

func (g *RouteGroup) Handle(route uint32, handler Handler) {
	g.router.add(route, handler, g)
}
