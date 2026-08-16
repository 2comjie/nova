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

type redirectError string

func (e redirectError) Error() string {
	return "redirect to " + string(e)
}

func (e redirectError) ErrorCode() uint32 {
	return rpc.ErrorCodeRedirect
}

func (e redirectError) ErrorDetail() []byte {
	return []byte(e)
}

func TestRPCReturnsActorRedirect(t *testing.T) {
	router := NewRouter()
	if err := router.Handle(1001, func(ctx *Context) error {
		if ctx.Request.ActorKey != "player:uid-1001" {
			t.Fatalf("actor key=%q", ctx.Request.ActorKey)
		}
		return redirectError("player-2")
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	nodeApp := &Node{
		instance: endpoint.ServiceInstance{ID: "player-1", ServiceName: "player"},
		router:   router,
	}
	request := &pbNode.Request{
		Uid:             "uid-1001",
		Route:           1001,
		GateServiceName: locator.GateName,
		GateInstanceId:  "gate-1",
		ActorKey:        "player:uid-1001",
	}

	_, err := nodeApp.Call(context.Background(), request)
	var redirect redirectError
	if !errors.As(err, &redirect) || redirect.ErrorCode() != rpc.ErrorCodeRedirect || string(redirect.ErrorDetail()) != "player-2" {
		t.Fatalf("call error=%v", err)
	}

	_, err = nodeApp.Tell(context.Background(), request)
	if !errors.As(err, &redirect) || redirect.ErrorCode() != rpc.ErrorCodeRedirect || string(redirect.ErrorDetail()) != "player-2" {
		t.Fatalf("tell error=%v", err)
	}
}
