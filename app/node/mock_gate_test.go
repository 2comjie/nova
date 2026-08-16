package node

import (
	"context"
	"testing"

	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
)

type mockCallGateClient struct {
	pbGate.GateClient
	strategy lx.Strategy
	request  *pbGate.MockCallRequest
}

func (c *mockCallGateClient) MockCall(ctx context.Context, request *pbGate.MockCallRequest, _ ...grpc.CallOption) (*pbGate.MockCallResponse, error) {
	c.strategy = lx.GetStrategy(ctx)
	c.request = request
	return &pbGate.MockCallResponse{Replied: true, Body: []byte{2}}, nil
}

func TestMockGateCallUsesGateService(t *testing.T) {
	client := &mockCallGateClient{}
	app := &Node{
		instance:   endpoint.ServiceInstance{ID: "api-1", ServiceName: "api"},
		gateClient: client,
	}
	body, replied, err := app.MockGateCall(context.Background(), "1001", 2001, []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if !replied || len(body) != 1 || body[0] != 2 {
		t.Fatalf("replied=%t body=%v", replied, body)
	}
	if client.strategy.Mode != lx.ModeBalance || client.strategy.Service != locator.GateName {
		t.Fatalf("strategy=%+v", client.strategy)
	}
	if client.request.Uid != "1001" || client.request.Route != 2001 || client.request.NodeServiceName != "api" || client.request.NodeInstanceId != "api-1" {
		t.Fatalf("request=%+v", client.request)
	}
}
