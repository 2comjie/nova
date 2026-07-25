package node

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestRouterGroupMiddlewareOrder(t *testing.T) {
	router := NewRouter()
	var calls []string

	if err := router.Use(recordMiddleware(&calls, "global")); err != nil {
		t.Fatal(err)
	}
	group := router.Group()
	if err := group.Use(recordMiddleware(&calls, "group")); err != nil {
		t.Fatal(err)
	}
	if err := group.Handle(1001, func(*Context) {
		calls = append(calls, "handler")
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := router.Dispatch(&Context{
		Context: context.Background(),
		Request: &Request{Route: 1001},
	}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"global-before",
		"group-before",
		"handler",
		"group-after",
		"global-after",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("调用顺序 = %v, want %v", calls, want)
	}
}

func TestRouteGroupMiddlewareOnlyAffectsLaterRoutes(t *testing.T) {
	router := NewRouter()
	group := router.Group()
	var firstCalls int
	var secondCalls int

	if err := group.Handle(1, func(*Context) {
		firstCalls++
	}); err != nil {
		t.Fatal(err)
	}
	if err := group.Use(func(next Handler) Handler {
		return func(ctx *Context) {
			secondCalls++
			next(ctx)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := group.Handle(2, func(*Context) {}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	if err := router.Dispatch(&Context{Request: &Request{Route: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := router.Dispatch(&Context{Request: &Request{Route: 2}}); err != nil {
		t.Fatal(err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("firstCalls=%d secondCalls=%d", firstCalls, secondCalls)
	}
}

func TestRouterRejectsInvalidRegistration(t *testing.T) {
	router := NewRouter()
	handler := func(*Context) {}

	if !errors.Is(router.Handle(0, handler), ErrInvalidRoute) {
		t.Fatal("route 0没有被拒绝")
	}
	if !errors.Is(router.Handle(1, nil), ErrNilHandler) {
		t.Fatal("nil Handler没有被拒绝")
	}
	if err := router.Handle(1, handler); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(router.Handle(1, handler), ErrRouteRegistered) {
		t.Fatal("重复route没有被拒绝")
	}
	if !errors.Is(router.Use(nil), ErrNilMiddleware) {
		t.Fatal("nil Middleware没有被拒绝")
	}
}

func TestRouterFreeze(t *testing.T) {
	router := NewRouter()
	if err := router.Handle(1, func(*Context) {}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatalf("重复Freeze失败: %v", err)
	}
	if !errors.Is(router.Handle(2, func(*Context) {}), ErrRouterFrozen) {
		t.Fatal("Freeze后仍然可以注册route")
	}
	if !errors.Is(router.Use(func(next Handler) Handler { return next }), ErrRouterFrozen) {
		t.Fatal("Freeze后仍然可以添加Middleware")
	}
	if !errors.Is(router.Group().Use(func(next Handler) Handler { return next }), ErrRouterFrozen) {
		t.Fatal("Freeze后RouteGroup仍然可以添加Middleware")
	}
}

func TestRouterDispatchErrors(t *testing.T) {
	router := NewRouter()
	if !errors.Is(router.Dispatch(&Context{Request: &Request{Route: 1}}), ErrRouterNotFrozen) {
		t.Fatal("未Freeze的Router没有返回错误")
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(router.Dispatch(nil), ErrInvalidContext) {
		t.Fatal("nil Context没有被拒绝")
	}
	if !errors.Is(router.Dispatch(&Context{Request: &Request{Route: 1}}), ErrRouteNotFound) {
		t.Fatal("未注册route没有返回错误")
	}
}

func TestRouterConcurrentDispatch(t *testing.T) {
	router := NewRouter()
	var mutex sync.Mutex
	count := 0
	if err := router.Handle(1, func(*Context) {
		mutex.Lock()
		count++
		mutex.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := router.Dispatch(&Context{Request: &Request{Route: 1}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()

	if count != 100 {
		t.Fatalf("count=%d, want 100", count)
	}
}

func recordMiddleware(calls *[]string, name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx *Context) {
			*calls = append(*calls, name+"-before")
			next(ctx)
			*calls = append(*calls, name+"-after")
		}
	}
}
