package node

import (
	"context"
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
	router.Handle(1001, func(ctx *Context) error {
		if ctx.Request.ActorKey != "player:uid-1001" {
			t.Fatalf("actor key=%q", ctx.Request.ActorKey)
		}
		return redirectError("player-2")
	})

	nodeApp := &Node{
		instance: endpoint.ServiceInstance{ID: "player-1", ServiceName: "player"},
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
	if err == nil || err.Code() != rpc.ErrorCodeRedirect || string(err.Detail()) != "player-2" {
		t.Fatalf("call error=%v", err)
	}

	_, err = nodeApp.Tell(context.Background(), request)
	if err == nil || err.Code() != rpc.ErrorCodeRedirect || string(err.Detail()) != "player-2" {
		t.Fatalf("tell error=%v", err)
	}
}
