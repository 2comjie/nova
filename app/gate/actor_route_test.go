package gate

import (
	"context"
	"errors"
	"testing"

	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/rpc"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
)

func TestValidateActorTarget(t *testing.T) {
	if err := validateTarget(&Target{Mode: RouteModeActor, Service: "player", ActorKeyResolver: "player"}); err != nil {
		t.Fatal(err)
	}
	if err := validateTarget(&Target{Mode: RouteModeActor}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("error=%v", err)
	}
}

type actorRouteNodeClient struct {
	calls    []lx.Strategy
	requests []*pbNode.Request
}

func (c *actorRouteNodeClient) Call(ctx context.Context, request *pbNode.Request, _ ...grpc.CallOption) (*pbNode.Response, error) {
	c.calls = append(c.calls, lx.GetStrategy(ctx))
	c.requests = append(c.requests, request)
	if len(c.calls) == 1 {
		return nil, rpc.NewErrorWithDetail(rpc.ErrorCodeRedirect, "actor redirect", []byte("player-2"))
	}
	return &pbNode.Response{NodeServiceName: "player", NodeInstanceId: "player-2", Replied: true, Body: []byte{7}}, nil
}

func (c *actorRouteNodeClient) Tell(ctx context.Context, request *pbNode.Request, _ ...grpc.CallOption) (*pbNode.Response, error) {
	c.calls = append(c.calls, lx.GetStrategy(ctx))
	c.requests = append(c.requests, request)
	if len(c.calls) == 1 {
		return nil, rpc.NewErrorWithDetail(rpc.ErrorCodeRedirect, "actor redirect", []byte("player-2"))
	}
	return &pbNode.Response{}, nil
}

func TestActorRouteRetriesOwner(t *testing.T) {
	for _, needReply := range []bool{true, false} {
		nodeClient := &actorRouteNodeClient{}
		gateApp := &Gate{
			instance:   endpoint.ServiceInstance{ID: "gate-1", ServiceName: "gate"},
			nodeClient: nodeClient,
		}
		ctx := &Context{
			Context:    context.Background(),
			App:        gateApp,
			Uid:        "uid-1001",
			Route:      1001,
			Target:     Target{Mode: RouteModeActor, Service: "player", ActorKeyResolver: "player"},
			BindingKey: "uid-1001",
			needReply:  needReply,
			actorKeyResolver: func(ctx *Context) string {
				return "player:" + ctx.BindingKey
			},
		}

		if err := gateApp.forward(ctx); err != nil {
			t.Fatal(err)
		}
		if len(nodeClient.calls) != 2 {
			t.Fatalf("calls=%d", len(nodeClient.calls))
		}
		if first := nodeClient.calls[0]; first.Mode != lx.ModeActor || first.Service != "player" || first.Key != "player:uid-1001" {
			t.Fatalf("first strategy=%+v", first)
		}
		if second := nodeClient.calls[1]; second.Mode != lx.ModeNode || second.Key != "player-2" {
			t.Fatalf("second strategy=%+v", second)
		}
		if nodeClient.requests[0].ActorKey != "player:uid-1001" || nodeClient.requests[1].ActorKey != "player:uid-1001" {
			t.Fatalf("actor key=%q/%q", nodeClient.requests[0].ActorKey, nodeClient.requests[1].ActorKey)
		}
		if needReply && (!ctx.replied || len(ctx.responseBody) != 1 || ctx.responseBody[0] != 7) {
			t.Fatalf("replied=%t body=%v", ctx.replied, ctx.responseBody)
		}
	}
}

func TestActorRouteRequiresRegisteredResolver(t *testing.T) {
	router := NewRouter()
	if err := router.Add(Route{
		ID:     "player",
		Routes: []uint32{1001},
		Target: Target{Mode: RouteModeActor, Service: "player", ActorKeyResolver: "player"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); !errors.Is(err, ErrActorKeyResolverNotFound) {
		t.Fatalf("freeze error=%v", err)
	}
}

func TestActorRouteResolverIsCompiled(t *testing.T) {
	router := NewRouter()
	if err := router.RegisterActorKeyResolver("player", func(ctx *Context) string {
		return "player:" + ctx.BindingKey
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(Route{
		ID:     "player",
		Routes: []uint32{1001},
		Target: Target{Mode: RouteModeActor, Service: "player", ActorKeyResolver: "player"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	called := false
	ctx := &Context{Context: context.Background(), Route: 1001, BindingKey: "uid-1001"}
	ctx.forward = func(ctx *Context) error {
		ctx.ActorKey = ctx.actorKeyResolver(ctx)
		called = true
		return nil
	}
	if err := router.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if !called || ctx.ActorKey != "player:uid-1001" {
		t.Fatalf("called=%t actorKey=%q", called, ctx.ActorKey)
	}
}

func TestMockCallUsesGateRouter(t *testing.T) {
	router := NewRouter()
	if err := router.RegisterActorKeyResolver("player", func(ctx *Context) string {
		return "player:" + ctx.Uid
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(Route{
		ID:     "player",
		Routes: []uint32{1001},
		Target: Target{Mode: RouteModeActor, Service: "player", ActorKeyResolver: "player"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	nodeClient := &actorRouteNodeClient{}
	gateApp := &Gate{
		instance:   endpoint.ServiceInstance{ID: "gate-1", ServiceName: "gate"},
		router:     router,
		nodeClient: nodeClient,
	}
	response, err := gateApp.MockCall(context.Background(), &pbGate.MockCallRequest{
		Uid:             "uid-1001",
		Route:           1001,
		Body:            []byte{1},
		NodeServiceName: "api",
		NodeInstanceId:  "api-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Replied || len(response.Body) != 1 || response.Body[0] != 7 {
		t.Fatalf("response=%+v", response)
	}
	if len(nodeClient.requests) != 2 || nodeClient.requests[0].Uid != "uid-1001" || nodeClient.requests[0].ActorKey != "player:uid-1001" {
		t.Fatalf("requests=%+v", nodeClient.requests)
	}
}
