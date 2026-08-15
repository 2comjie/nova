package gate

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/rpc/lx"
)

type RouteMode string

const (
	RouteModeBalance RouteMode = "balance"
	RouteModeSelect  RouteMode = "select"
	RouteModeNode    RouteMode = "node"
	RouteModeActor   RouteMode = "actor"
)

var (
	ErrInvalidRoute               = errors.New("gate: route必须大于0")
	ErrRouteRegistered            = errors.New("gate: route已经注册")
	ErrRouteIDRegistered          = errors.New("gate: Route ID已经注册")
	ErrRouteNotFound              = errors.New("gate: route不存在")
	ErrInvalidTarget              = errors.New("gate: Target无效")
	ErrNilFilter                  = errors.New("gate: Filter不能为空")
	ErrNilFilterFactory           = errors.New("gate: FilterFactory不能为空")
	ErrFilterRegistered           = errors.New("gate: FilterFactory已经注册")
	ErrFilterNotFound             = errors.New("gate: FilterFactory不存在")
	ErrActorKeyResolverRegistered = errors.New("gate: ActorKeyResolver已经注册")
	ErrActorKeyResolverNotFound   = errors.New("gate: ActorKeyResolver不存在")
	ErrRouterFrozen               = errors.New("gate: Router已经冻结")
	ErrRouterNotFrozen            = errors.New("gate: Router尚未冻结")
	ErrInvalidFilterName          = errors.New("gate: Filter名称不能为空")
	ErrInvalidRouteID             = errors.New("gate: Route ID不能为空")
	ErrRouteWithoutMatchers       = errors.New("gate: Route没有配置数字route")
	ErrActorRedirect              = errors.New("gate: Actor重定向失败")
)

type Handler func(*Context) error
type Filter func(ctx *Context, next Handler) error
type FilterFactory func(args map[string]string) (Filter, error)
type ActorKeyResolver func(ctx *Context) string
type FilterConfig struct {
	Name string            `json:"name" yaml:"name"`
	Args map[string]string `json:"args" yaml:"args"`
}

type Target struct {
	Mode             RouteMode        `json:"mode" yaml:"mode"`
	Service          string           `json:"service" yaml:"service"`
	Binding          string           `json:"binding" yaml:"binding"`
	Balance          lx.BalancePolicy `json:"balance" yaml:"balance"`
	NodeID           string           `json:"node_id" yaml:"node_id"`
	ActorKeyResolver string           `json:"actor_key_resolver" yaml:"actor_key_resolver"`
}

type Route struct {
	ID      string         `json:"id" yaml:"id"`
	Routes  []uint32       `json:"routes" yaml:"routes"`
	Filters []FilterConfig `json:"filters" yaml:"filters"`
	Target  Target         `json:"target" yaml:"target"`
}

type compiledRoute struct {
	id               string
	target           Target
	actorKeyResolver ActorKeyResolver
	handler          Handler
}

type routeTable map[uint32]compiledRoute

type Router struct {
	mutex             sync.Mutex
	routes            []Route
	filters           []FilterConfig
	factories         map[string]FilterFactory
	actorKeyResolvers map[string]ActorKeyResolver
	frozen            bool
	table             atomic.Pointer[routeTable]
}

func NewRouter() *Router {
	return &Router{
		factories:         make(map[string]FilterFactory),
		actorKeyResolvers: make(map[string]ActorKeyResolver),
	}
}

func (r *Router) RegisterActorKeyResolver(name string, resolver ActorKeyResolver) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return ErrRouterFrozen
	}
	if r.actorKeyResolvers[name] != nil {
		return fmt.Errorf("%w: %s", ErrActorKeyResolverRegistered, name)
	}
	r.actorKeyResolvers[name] = resolver
	return nil
}

func (r *Router) RegisterFilter(name string, factory FilterFactory) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidFilterName
	}
	if factory == nil {
		return ErrNilFilterFactory
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return ErrRouterFrozen
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("%w: %s", ErrFilterRegistered, name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Router) Use(filters ...FilterConfig) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return ErrRouterFrozen
	}
	for _, filter := range filters {
		r.filters = append(r.filters, cloneFilterConfig(filter))
	}
	return nil
}

func (r *Router) Add(routes ...Route) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return ErrRouterFrozen
	}
	for _, route := range routes {
		r.routes = append(r.routes, cloneRoute(route))
	}
	return nil
}

func (r *Router) Freeze() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.frozen {
		return nil
	}

	globalFilters, err := r.compileFilters(r.filters)
	if err != nil {
		return fmt.Errorf("gate: 编译全局Filter失败: %w", err)
	}

	table := make(routeTable)
	routeIDs := make(map[string]struct{}, len(r.routes))
	for _, route := range r.routes {
		if strings.TrimSpace(route.ID) == "" {
			return ErrInvalidRouteID
		}
		if _, exists := routeIDs[route.ID]; exists {
			return fmt.Errorf("%w: %s", ErrRouteIDRegistered, route.ID)
		}
		routeIDs[route.ID] = struct{}{}
		if len(route.Routes) == 0 {
			return fmt.Errorf("%w: %s", ErrRouteWithoutMatchers, route.ID)
		}
		if err := validateTarget(&route.Target); err != nil {
			return fmt.Errorf("gate: Route %s: %w", route.ID, err)
		}
		var actorKeyResolver ActorKeyResolver
		if route.Target.Mode == RouteModeActor {
			actorKeyResolver = r.actorKeyResolvers[route.Target.ActorKeyResolver]
			if actorKeyResolver == nil {
				return fmt.Errorf("gate: Route %s: %w: %s", route.ID, ErrActorKeyResolverNotFound, route.Target.ActorKeyResolver)
			}
		}

		routeFilters, err := r.compileFilters(route.Filters)
		if err != nil {
			return fmt.Errorf("gate: 编译Route %s Filter失败: %w", route.ID, err)
		}
		filters := make([]Filter, 0, len(globalFilters)+len(routeFilters))
		filters = append(filters, globalFilters...)
		filters = append(filters, routeFilters...)

		handler := Handler(func(ctx *Context) error {
			return ctx.forward(ctx)
		})
		for index := len(filters) - 1; index >= 0; index-- {
			filter := filters[index]
			next := handler
			handler = func(ctx *Context) error {
				return filter(ctx, next)
			}
		}

		entry := compiledRoute{
			id:               route.ID,
			target:           route.Target,
			actorKeyResolver: actorKeyResolver,
			handler:          handler,
		}
		for _, routeNumber := range route.Routes {
			if routeNumber == 0 {
				return fmt.Errorf("%w: Route %s", ErrInvalidRoute, route.ID)
			}
			if _, exists := table[routeNumber]; exists {
				return fmt.Errorf("%w: %d", ErrRouteRegistered, routeNumber)
			}
			table[routeNumber] = entry
		}
	}

	r.table.Store(&table)
	r.routes = nil
	r.filters = nil
	r.factories = nil
	r.actorKeyResolvers = nil
	r.frozen = true
	return nil
}

func (r *Router) Dispatch(ctx *Context) error {
	table := r.table.Load()
	if table == nil {
		return ErrRouterNotFrozen
	}
	routeNumber := ctx.Route
	route, exists := (*table)[routeNumber]
	if !exists {
		return fmt.Errorf("%w: %d", ErrRouteNotFound, routeNumber)
	}

	ctx.RouteID = route.id
	ctx.Target = route.target
	ctx.actorKeyResolver = route.actorKeyResolver
	return route.handler(ctx)
}

func (r *Router) compileFilters(configs []FilterConfig) ([]Filter, error) {
	filters := make([]Filter, 0, len(configs))
	for _, config := range configs {
		name := strings.TrimSpace(config.Name)
		if name == "" {
			return nil, ErrInvalidFilterName
		}
		factory := r.factories[name]
		if factory == nil {
			return nil, fmt.Errorf("%w: %s", ErrFilterNotFound, name)
		}
		filter, err := factory(cloneStringMap(config.Args))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if filter == nil {
			return nil, fmt.Errorf("%w: %s", ErrNilFilter, name)
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

func validateTarget(target *Target) error {
	switch target.Mode {
	case RouteModeBalance:
		if target.Service == "" {
			return ErrInvalidTarget
		}
		if target.Balance == "" {
			target.Balance = lx.BalanceWeightedRoundRobin
		}
	case RouteModeSelect:
		if target.Service == "" || target.Binding == "" {
			return ErrInvalidTarget
		}
	case RouteModeActor:
		if target.Service == "" || target.ActorKeyResolver == "" {
			return ErrInvalidTarget
		}
		if target.Balance == "" {
			target.Balance = lx.BalanceWeightedRoundRobin
		}
	case RouteModeNode:
		if target.NodeID == "" {
			return ErrInvalidTarget
		}
	default:
		return ErrInvalidTarget
	}
	return nil
}

func cloneRoute(route Route) Route {
	route.Routes = append([]uint32(nil), route.Routes...)
	filters := make([]FilterConfig, 0, len(route.Filters))
	for _, filter := range route.Filters {
		filters = append(filters, cloneFilterConfig(filter))
	}
	route.Filters = filters
	return route
}

func cloneFilterConfig(config FilterConfig) FilterConfig {
	config.Args = cloneStringMap(config.Args)
	return config
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	target := make(map[string]string, len(source))
	maps.Copy(target, source)
	return target
}
