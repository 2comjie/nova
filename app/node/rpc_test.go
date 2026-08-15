package node

import (
	"context"
	"testing"

	"github.com/2comjie/wali/core/endpoint"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
)

type redirectError string

func (e redirectError) Error() string {
	return "redirect to " + string(e)
}

func (e redirectError) RedirectInstanceId() string {
	return string(e)
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

	response, err := nodeApp.Call(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.RedirectInstanceId != "player-2" {
		t.Fatalf("call redirect=%s", response.RedirectInstanceId)
	}

	response, err = nodeApp.Tell(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.RedirectInstanceId != "player-2" {
		t.Fatalf("tell redirect=%s", response.RedirectInstanceId)
	}
}
