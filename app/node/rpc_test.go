package node

import (
	"context"
	"errors"
	"testing"

	"github.com/2comjie/nova/core/endpoint"
	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/rpc"
)

func TestRPCReturnsActorRedirect(t *testing.T) {
	router := NewRouter()
	router.Handle(1001, func(ctx *Context) error {
		if ctx.Request.ActorKey != "player:uid-1001" {
			t.Fatalf("actor key=%q", ctx.Request.ActorKey)
		}
		return rpc.NewErrorWithDetail(rpc.ErrorCodeRedirect, "redirect", []byte("player-2"))
	})

	nodeApp := &Node{
		instance: endpoint.ServiceInstance{Id: "player-1", ServiceName: "player"},
		router:   router,
	}
	request := &pbNode.Request{
		Uid:             1001,
		Route:           1001,
		GateServiceName: locator.GateName,
		GateInstanceId:  "gate-1",
		ActorKey:        "player:uid-1001",
	}

	_, err := nodeApp.Call(context.Background(), request)
	var redirect *rpc.Error
	if !errors.As(err, &redirect) || redirect.Code != rpc.ErrorCodeRedirect || string(redirect.Detail) != "player-2" {
		t.Fatalf("call error=%v", err)
	}

	_, err = nodeApp.Tell(context.Background(), request)
	if !errors.As(err, &redirect) || redirect.Code != rpc.ErrorCodeRedirect || string(redirect.Detail) != "player-2" {
		t.Fatalf("tell error=%v", err)
	}
}

func TestRPCConvertsHandlerErrorAtServerBoundary(t *testing.T) {
	nodeApp := &Node{router: NewRouter()}
	request := &pbNode.Request{Uid: 1, Route: 1, GateServiceName: locator.GateName, GateInstanceId: "gate-1"}
	for _, test := range []struct {
		err  error
		code uint32
	}{
		{context.Canceled, rpc.CodeCanceled},
		{context.DeadlineExceeded, rpc.CodeDeadlineExceeded},
		{errors.New("handler failed"), rpc.CodeInternal},
	} {
		nodeApp.router.Handle(1, func(*Context) error { return test.err })
		_, err := nodeApp.Call(context.Background(), request)
		if err == nil || err.Code != test.code || err.Error() != test.err.Error() {
			t.Fatalf("Call error=%v, want status %v", err, test.code)
		}
		_, err = nodeApp.Tell(context.Background(), request)
		if err == nil || err.Code != test.code || err.Error() != test.err.Error() {
			t.Fatalf("Tell error=%v, want status %v", err, test.code)
		}
	}
	want := rpc.NewError(1001, "business failure")
	nodeApp.router.Handle(1, func(*Context) error { return want })
	if _, err := nodeApp.Call(context.Background(), request); err != want {
		t.Fatalf("business error=%v, want original %v", err, want)
	}
	request.Route = 2
	_, err := nodeApp.Call(context.Background(), request)
	if err == nil || err.Code != rpc.CodeNotFound {
		t.Fatalf("missing route error=%v", err)
	}
}

func TestRPCDoesNotRecoverHandlerPanic(t *testing.T) {
	nodeApp := &Node{router: NewRouter()}
	nodeApp.router.Handle(1, func(*Context) error { panic("broken actor state") })
	defer func() {
		if value := recover(); value != "broken actor state" {
			t.Fatalf("panic=%v", value)
		}
	}()
	_, _ = nodeApp.Call(context.Background(), &pbNode.Request{
		Uid: 1, Route: 1, GateServiceName: locator.GateName, GateInstanceId: "gate-1",
	})
}
