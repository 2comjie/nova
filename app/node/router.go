package node

import (
	"errors"
	"fmt"
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

// Handler 处理一个 Node 请求。
type Handler func(*Context)

// Middleware 包装下一个 Handler，可以在调用前后执行逻辑，也可以不调用 next 来终止请求。
type Middleware func(next Handler) Handler

type routeEntry struct {
	handler     Handler
	middlewares []Middleware
}

type routeTable map[uint32]Handler

// Router 保存 Node 本地的 route、Handler 和 Middleware。
// Router 在 Freeze 前注册，Freeze 后只允许并发 Dispatch。
type Router struct {
	mutex      sync.Mutex
	routes     map[uint32]routeEntry
	middleware []Middleware
	frozen     bool
	table      atomic.Pointer[routeTable]
}

// NewRouter 创建 Node Router。
func NewRouter() *Router {
	return &Router{
		routes: make(map[uint32]routeEntry),
	}
}

// Use 添加所有路由共用的 Middleware。
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

// Handle 注册一个路由。
func (r *Router) Handle(route uint32, handler Handler) error {
	return r.add(route, handler, nil)
}

// Group 创建共享 Middleware 的路由组。
func (r *Router) Group() *RouteGroup {
	return &RouteGroup{router: r}
}

// Freeze 完成 Middleware 调用链并冻结 Router。
// Freeze 成功后可以被重复调用。
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
			return fmt.Errorf("node: 编译route %d失败: %w", route, err)
		}
		table[route] = handler
	}

	r.table.Store(&table)
	r.frozen = true
	return nil
}

// Dispatch 查找并执行当前请求的 Handler。
func (r *Router) Dispatch(ctx *Context) error {
	table := r.table.Load()
	if table == nil {
		return ErrRouterNotFrozen
	}
	handler, ok := (*table)[ctx.Request.Route]
	if !ok {
		return fmt.Errorf("%w: %d", ErrRouteNotFound, ctx.Request.Route)
	}
	handler(ctx)
	return nil
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
		return fmt.Errorf("%w: %d", ErrRouteRegistered, route)
	}
	var middlewares []Middleware
	if group != nil {
		middlewares = append(middlewares, group.middlewares...)
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

// RouteGroup 保存一组路由共用的 Middleware。
type RouteGroup struct {
	router      *Router
	middlewares []Middleware
}

// Use 添加当前组后续注册路由使用的 Middleware。
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

// Handle 使用当前组的 Middleware 注册路由。
func (g *RouteGroup) Handle(route uint32, handler Handler) error {
	return g.router.add(route, handler, g)
}
