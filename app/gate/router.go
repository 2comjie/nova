package gate

import (
	"errors"

	"github.com/2comjie/nova/rpc/lx"
)

type RouteMode string

const (
	RouteModeBalance RouteMode = "balance"
	RouteModeSelect  RouteMode = "select"
	RouteModeNode    RouteMode = "node"
	RouteModeActor   RouteMode = "actor"
)

var (
	ErrInvalidRoute             = errors.New("gate: route必须大于0")
	ErrRouteNotFound            = errors.New("gate: route不存在")
	ErrInvalidTarget            = errors.New("gate: Target无效")
	ErrNilFilter                = errors.New("gate: Filter不能为空")
	ErrFilterNotFound           = errors.New("gate: FilterFactory不存在")
	ErrActorKeyResolverNotFound = errors.New("gate: ActorKeyResolver不存在")
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

type Router struct {
	routes            map[uint32]compiledRoute
	filters           []Filter
	factories         map[string]FilterFactory
	actorKeyResolvers map[string]ActorKeyResolver
}

func NewRouter() *Router {
	return &Router{
		routes:            make(map[uint32]compiledRoute),
		factories:         make(map[string]FilterFactory),
		actorKeyResolvers: make(map[string]ActorKeyResolver),
	}
}

func (r *Router) RegisterActorKeyResolver(name string, resolver ActorKeyResolver) {
	r.actorKeyResolvers[name] = resolver
}

func (r *Router) RegisterFilter(name string, factory FilterFactory) {
	r.factories[name] = factory
}

func (r *Router) Use(configs ...FilterConfig) {
	r.filters = append(r.filters, r.compileFilters(configs)...)
}

func (r *Router) Add(routes ...Route) {
	for _, route := range routes {
		if err := validateTarget(&route.Target); err != nil {
			panic(err)
		}

		var actorKeyResolver ActorKeyResolver
		if route.Target.Mode == RouteModeActor {
			actorKeyResolver = r.actorKeyResolvers[route.Target.ActorKeyResolver]
			if actorKeyResolver == nil {
				panic(ErrActorKeyResolverNotFound)
			}
		}

		filters := append([]Filter(nil), r.filters...)
		filters = append(filters, r.compileFilters(route.Filters)...)
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
				panic(ErrInvalidRoute)
			}
			r.routes[routeNumber] = entry
		}
	}
}

func (r *Router) Dispatch(ctx *Context) error {
	route, exists := r.routes[ctx.Route]
	if !exists {
		return ErrRouteNotFound
	}
	ctx.RouteID = route.id
	ctx.Target = route.target
	ctx.actorKeyResolver = route.actorKeyResolver
	return route.handler(ctx)
}

func (r *Router) compileFilters(configs []FilterConfig) []Filter {
	filters := make([]Filter, 0, len(configs))
	for _, config := range configs {
		factory := r.factories[config.Name]
		if factory == nil {
			panic(ErrFilterNotFound)
		}
		filter, err := factory(config.Args)
		if err != nil {
			panic(err)
		}
		if filter == nil {
			panic(ErrNilFilter)
		}
		filters = append(filters, filter)
	}
	return filters
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
