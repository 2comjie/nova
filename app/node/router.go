package node

import (
	"errors"
	"sync"
	"sync/atomic"
)

var (
	ErrInvalidRoute    = errors.New("node: route必须大于0")
	ErrRouteRegistered = errors.New("node: route已经注册")
	ErrNilHandler      = errors.New("node: Handler不能为空")
	ErrNilMiddleware   = errors.New("node: Middleware不能为空")
	ErrRouterFrozen    = errors.New("node: Router已经冻结")
	ErrRouterNotFrozen = errors.New("node: Router尚未冻结")
	ErrRouteNotFound   = errors.New("node: route不存在")
)

type Handler func(*Context) error

type Middleware func(next Handler) Handler

type routeEntry struct {
	handler     Handler
	middlewares []Middleware
}

type routeTable map[uint32]Handler

type Router struct {
	mutex      sync.Mutex
	routes     map[uint32]routeEntry
	middleware []Middleware
	frozen     bool
	table      atomic.Pointer[routeTable]
}

func NewRouter() *Router {
	return &Router{
		routes: make(map[uint32]routeEntry),
	}
}

func (r *Router) Use(middlewares ...Middleware) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return ErrRouterFrozen
	}
	for _, middleware := range middlewares {
		if middleware == nil {
			return ErrNilMiddleware
		}
	}
	r.middleware = append(r.middleware, middlewares...)
	return nil
}

func (r *Router) Handle(route uint32, handler Handler) error {
	return r.add(route, handler, nil)
}

func (r *Router) Group() *RouteGroup {
	return &RouteGroup{router: r}
}

func (r *Router) Freeze() (err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return nil
	}

	table := make(routeTable, len(r.routes))
	for route, entry := range r.routes {
		handler := entry.handler
		middlewares := make([]Middleware, 0, len(r.middleware)+len(entry.middlewares))
		middlewares = append(middlewares, r.middleware...)
		middlewares = append(middlewares, entry.middlewares...)

		handler, err = buildHandler(handler, middlewares)
		if err != nil {
			return err
		}
		table[route] = handler
	}

	r.table.Store(&table)
	r.frozen = true
	return nil
}

func (r *Router) Dispatch(ctx *Context) error {
	table := r.table.Load()
	if table == nil {
		return ErrRouterNotFrozen
	}
	handler, ok := (*table)[ctx.Request.Route]
	if !ok {
		return ErrRouteNotFound
	}
	return handler(ctx)
}

func (r *Router) add(route uint32, handler Handler, group *RouteGroup) error {
	if route == 0 {
		return ErrInvalidRoute
	}
	if handler == nil {
		return ErrNilHandler
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return ErrRouterFrozen
	}
	if _, exists := r.routes[route]; exists {
		return ErrRouteRegistered
	}
	var middlewares []Middleware
	if group != nil {
		groups := make([]*RouteGroup, 0, 2)
		for current := group; current != nil; current = current.parent {
			groups = append(groups, current)
		}
		for index := len(groups) - 1; index >= 0; index-- {
			middlewares = append(middlewares, groups[index].middlewares...)
		}
	}
	r.routes[route] = routeEntry{
		handler:     handler,
		middlewares: middlewares,
	}
	return nil
}

func buildHandler(handler Handler, middlewares []Middleware) (result Handler, err error) {
	result = handler
	for index := len(middlewares) - 1; index >= 0; index-- {
		result = middlewares[index](result)
		if result == nil {
			return nil, ErrNilHandler
		}
	}
	return result, nil
}

type RouteGroup struct {
	router      *Router
	parent      *RouteGroup
	middlewares []Middleware
}

func (g *RouteGroup) Group() *RouteGroup {
	return &RouteGroup{router: g.router, parent: g}
}

func (g *RouteGroup) Use(middlewares ...Middleware) error {
	for _, middleware := range middlewares {
		if middleware == nil {
			return ErrNilMiddleware
		}
	}

	g.router.mutex.Lock()
	defer g.router.mutex.Unlock()
	if g.router.frozen {
		return ErrRouterFrozen
	}
	g.middlewares = append(g.middlewares, middlewares...)
	return nil
}

func (g *RouteGroup) Handle(route uint32, handler Handler) error {
	return g.router.add(route, handler, g)
}
