package gate

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/2comjie/wali/rpc/lx"
)

func TestRouterFilterOrder(t *testing.T) {
	router := NewRouter()
	var calls []string

	if err := router.RegisterFilter("record", func(args map[string]string) (Filter, error) {
		name := args["name"]
		return func(ctx *Context, next Handler) error {
			calls = append(calls, name+"-before")
			err := next(ctx)
			calls = append(calls, name+"-after")
			return err
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Use(FilterConfig{
		Name: "record",
		Args: map[string]string{"name": "global"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(Route{
		ID:     "player",
		Routes: []uint32{1001},
		Filters: []FilterConfig{{
			Name: "record",
			Args: map[string]string{"name": "route"},
		}},
		Target: Target{
			Mode:    RouteModeBalance,
			Service: "player",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	ctx := testContext(1001)
	ctx.forward = func(*Context) error {
		calls = append(calls, "forward")
		return nil
	}
	if err := router.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"global-before",
		"route-before",
		"forward",
		"route-after",
		"global-after",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v, want %v", calls, want)
	}
	if ctx.RouteID != "player" {
		t.Fatalf("RouteID=%q", ctx.RouteID)
	}
	if ctx.Target.Balance != lx.BalanceWeightedRoundRobin {
		t.Fatalf("Balance=%q", ctx.Target.Balance)
	}
}

func TestRouterFilterCanStopForward(t *testing.T) {
	router := NewRouter()
	if err := router.RegisterFilter("stop", func(map[string]string) (Filter, error) {
		return func(ctx *Context, next Handler) error {
			return ctx.Reply([]byte("maintenance"))
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(Route{
		ID:      "maintenance",
		Routes:  []uint32{1},
		Filters: []FilterConfig{{Name: "stop"}},
		Target:  Target{Mode: RouteModeBalance, Service: "player"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	forwarded := false
	ctx := testContext(1)
	ctx.forward = func(*Context) error {
		forwarded = true
		return nil
	}
	if err := router.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if forwarded {
		t.Fatal("被Filter终止的请求仍然执行了Forward")
	}
	if !ctx.replied || string(ctx.responseBody) != "maintenance" {
		t.Fatalf("响应结果不正确: replied=%v body=%q", ctx.replied, ctx.responseBody)
	}
}

func TestRouterTargetIsCopiedPerRequest(t *testing.T) {
	router := NewRouter()
	if err := router.RegisterFilter("gray", func(map[string]string) (Filter, error) {
		return func(ctx *Context, next Handler) error {
			if ctx.Seq == 1 {
				ctx.Target.Service = "player-gray"
			}
			return next(ctx)
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(Route{
		ID:      "player",
		Routes:  []uint32{1},
		Filters: []FilterConfig{{Name: "gray"}},
		Target:  Target{Mode: RouteModeBalance, Service: "player"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	first := testContext(1)
	first.Seq = 1
	first.forward = func(*Context) error { return nil }
	if err := router.Dispatch(first); err != nil {
		t.Fatal(err)
	}

	second := testContext(1)
	second.Seq = 2
	second.forward = func(*Context) error { return nil }
	if err := router.Dispatch(second); err != nil {
		t.Fatal(err)
	}
	if first.Target.Service != "player-gray" || second.Target.Service != "player" {
		t.Fatalf("Target被请求间共享: first=%q second=%q", first.Target.Service, second.Target.Service)
	}
}

func TestRouterFreezeValidation(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Router)
		target error
	}{
		{
			name: "重复数字route",
			setup: func(router *Router) {
				_ = router.Add(
					Route{ID: "a", Routes: []uint32{1}, Target: Target{Mode: RouteModeBalance, Service: "a"}},
					Route{ID: "b", Routes: []uint32{1}, Target: Target{Mode: RouteModeBalance, Service: "b"}},
				)
			},
			target: ErrRouteRegistered,
		},
		{
			name: "重复RouteID",
			setup: func(router *Router) {
				_ = router.Add(
					Route{ID: "same", Routes: []uint32{1}, Target: Target{Mode: RouteModeBalance, Service: "a"}},
					Route{ID: "same", Routes: []uint32{2}, Target: Target{Mode: RouteModeBalance, Service: "b"}},
				)
			},
			target: ErrRouteIDRegistered,
		},
		{
			name: "Filter不存在",
			setup: func(router *Router) {
				_ = router.Add(Route{
					ID:      "a",
					Routes:  []uint32{1},
					Filters: []FilterConfig{{Name: "missing"}},
					Target:  Target{Mode: RouteModeBalance, Service: "a"},
				})
			},
			target: ErrFilterNotFound,
		},
		{
			name: "Target无效",
			setup: func(router *Router) {
				_ = router.Add(Route{
					ID:     "a",
					Routes: []uint32{1},
					Target: Target{Mode: RouteModeSelect},
				})
			},
			target: ErrInvalidTarget,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter()
			test.setup(router)
			if !errors.Is(router.Freeze(), test.target) {
				t.Fatalf("Freeze错误没有匹配%v", test.target)
			}
		})
	}
}

func TestRouterFreezeRejectsMutationAndSupportsConcurrentDispatch(t *testing.T) {
	router := NewRouter()
	if err := router.Add(Route{
		ID:     "player",
		Routes: []uint32{1},
		Target: Target{Mode: RouteModeBalance, Service: "player"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(router.Add(Route{}), ErrRouterFrozen) {
		t.Fatal("Freeze后仍然可以添加Route")
	}
	if !errors.Is(router.Use(FilterConfig{}), ErrRouterFrozen) {
		t.Fatal("Freeze后仍然可以添加全局Filter")
	}
	if !errors.Is(router.RegisterFilter("x", func(map[string]string) (Filter, error) {
		return nil, nil
	}), ErrRouterFrozen) {
		t.Fatal("Freeze后仍然可以注册Filter")
	}

	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ctx := testContext(1)
			ctx.forward = func(*Context) error { return nil }
			if err := router.Dispatch(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
}

func TestRouterDispatchErrors(t *testing.T) {
	router := NewRouter()
	if !errors.Is(router.Dispatch(testContext(1)), ErrRouterNotFrozen) {
		t.Fatal("未Freeze没有返回错误")
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(router.Dispatch(testContext(1)), ErrRouteNotFound) {
		t.Fatal("未配置route没有返回错误")
	}
}

func testContext(route uint32) *Context {
	return &Context{Route: route, needReply: true}
}
