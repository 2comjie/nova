package node

import (
	"context"
	"errors"
	"testing"

	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestContextReply(t *testing.T) {
	tell := &Context{Request: &Request{}}
	if !errors.Is(tell.Reply([]byte("rsp")), ErrReplyNotAllowed) {
		t.Fatal("Tell请求可以调用Reply")
	}

	call := &Context{Request: &Request{NeedReply: true}}
	body := []byte("rsp")
	if err := call.Reply(body); err != nil {
		t.Fatal(err)
	}
	if !call.replied || string(call.responseBody) != "rsp" {
		t.Fatalf("响应没有保存: replied=%v body=%q", call.replied, call.responseBody)
	}
	if !errors.Is(call.Reply(body), ErrAlreadyReplied) {
		t.Fatal("同一请求可以重复调用Reply")
	}
}

func TestProxy(t *testing.T) {
	provider := newTestLocatorProvider()
	gateLocator := locator.NewGateLocator(provider, nil)
	nodeLocator := locator.NewNodeLocator(provider)
	gateClient := &testGateClient{}
	app := &Node{
		instance: endpoint.ServiceInstance{
			ID:          "room-1",
			ServiceName: "room",
			MetaData:    map[string]string{"zone": "1"},
		},
		nodeLocator: nodeLocator,
		gateLocator: gateLocator,
		gateClient:  gateClient,
	}
	proxy := &Proxy{app: app}
	ctx := context.Background()

	if err := gateLocator.Bind(ctx, "user-1", "gate-1"); err != nil {
		t.Fatal(err)
	}
	if err := proxy.Bind(ctx, "team", "team-100"); err != nil {
		t.Fatal(err)
	}
	if err := proxy.Bind(ctx, "lobby", "user-1"); err != nil {
		t.Fatal(err)
	}
	instanceID, err := proxy.Locate(ctx, "team", "team-100")
	if err != nil {
		t.Fatal(err)
	}
	if instanceID != "room-1" {
		t.Fatalf("Locate=%q, want room-1", instanceID)
	}
	instanceID, err = proxy.Locate(ctx, "lobby", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if instanceID != "room-1" {
		t.Fatalf("Locate=%q, want room-1", instanceID)
	}

	if err := proxy.Push(ctx, "user-1", 1001, []byte("push")); err != nil {
		t.Fatal(err)
	}
	if gateClient.push == nil ||
		gateClient.push.Uid != "user-1" ||
		gateClient.push.Route != 1001 ||
		gateClient.push.NodeServiceName != "room" ||
		gateClient.push.NodeInstanceId != "room-1" {
		t.Fatalf("Push请求错误: %+v", gateClient.push)
	}
	if gateClient.pushStrategy.Mode != lx.ModeNode || gateClient.pushStrategy.Key != "gate-1" {
		t.Fatalf("Push没有定向到玩家所在Gate: %+v", gateClient.pushStrategy)
	}

	if err := proxy.Kick(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if gateClient.kick == nil ||
		gateClient.kick.Uid != "user-1" ||
		gateClient.kick.NodeServiceName != "room" ||
		gateClient.kick.NodeInstanceId != "room-1" {
		t.Fatalf("Kick请求错误: %+v", gateClient.kick)
	}
	if gateClient.kickStrategy.Mode != lx.ModeNode || gateClient.kickStrategy.Key != "gate-1" {
		t.Fatalf("Kick没有定向到玩家所在Gate: %+v", gateClient.kickStrategy)
	}

	instance := proxy.Instance()
	instance.MetaData["zone"] = "2"
	if proxy.app.instance.MetaData["zone"] != "1" {
		t.Fatal("业务层可以修改Node内部的实例元数据")
	}

	if err := proxy.Unbind(ctx, "team", "team-100"); err != nil {
		t.Fatal(err)
	}
	instanceID, err = proxy.Locate(ctx, "team", "team-100")
	if err != nil {
		t.Fatal(err)
	}
	if instanceID != "" {
		t.Fatalf("Unbind后Locate=%q", instanceID)
	}
}

type testLocatorProvider struct {
	bindings map[string]string
}

func newTestLocatorProvider() *testLocatorProvider {
	return &testLocatorProvider{bindings: make(map[string]string)}
}

func (p *testLocatorProvider) Bind(
	_ context.Context,
	name string,
	key string,
	instanceID string,
) error {
	p.bindings[name+":"+key] = instanceID
	return nil
}

func (p *testLocatorProvider) Unbind(
	_ context.Context,
	name string,
	key string,
	instanceID string,
) error {
	binding := name + ":" + key
	if p.bindings[binding] == instanceID {
		delete(p.bindings, binding)
	}
	return nil
}

func (p *testLocatorProvider) Locate(
	_ context.Context,
	name string,
	key string,
) (string, error) {
	return p.bindings[name+":"+key], nil
}

func (p *testLocatorProvider) Close() {}

type testGateClient struct {
	push         *pbGate.PushRequest
	kick         *pbGate.KickRequest
	pushStrategy lx.Strategy
	kickStrategy lx.Strategy
}

func (c *testGateClient) Push(
	ctx context.Context,
	request *pbGate.PushRequest,
	_ ...grpc.CallOption,
) (*emptypb.Empty, error) {
	c.push = request
	c.pushStrategy = lx.GetStrategy(ctx)
	return &emptypb.Empty{}, nil
}

func (c *testGateClient) Kick(
	ctx context.Context,
	request *pbGate.KickRequest,
	_ ...grpc.CallOption,
) (*emptypb.Empty, error) {
	c.kick = request
	c.kickStrategy = lx.GetStrategy(ctx)
	return &emptypb.Empty{}, nil
}
