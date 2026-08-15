package node

import (
	"errors"
	"reflect"
	"testing"
)

func TestRouterReturnsHandlerError(t *testing.T) {
	handlerErr := errors.New("handler failed")
	router := NewRouter()
	if err := router.Handle(1, func(*Context) error {
		return handlerErr
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	err := router.Dispatch(&Context{Request: &Request{Route: 1}})
	if !errors.Is(err, handlerErr) {
		t.Fatalf("dispatch error=%v", err)
	}
}

func TestRouterMiddlewareOrder(t *testing.T) {
	router := NewRouter()
	var calls []string
	first := func(next Handler) Handler {
		return func(ctx *Context) error {
			calls = append(calls, "first-before")
			err := next(ctx)
			calls = append(calls, "first-after")
			return err
		}
	}
	second := func(next Handler) Handler {
		return func(ctx *Context) error {
			calls = append(calls, "second-before")
			err := next(ctx)
			calls = append(calls, "second-after")
			return err
		}
	}
	if err := router.Use(first, second); err != nil {
		t.Fatal(err)
	}
	if err := router.Handle(1, func(*Context) error {
		calls = append(calls, "handler")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := router.Dispatch(&Context{Request: &Request{Route: 1}}); err != nil {
		t.Fatal(err)
	}

	want := []string{"first-before", "second-before", "handler", "second-after", "first-after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v", calls)
	}
}
