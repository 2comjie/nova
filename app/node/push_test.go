package node

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/2comjie/nova/core/endpoint"
	pbGate "github.com/2comjie/nova/internal/pb/transport/gate"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/network"
	"github.com/2comjie/nova/registry"
	"github.com/2comjie/nova/rpc/lx"
	"google.golang.org/protobuf/types/known/emptypb"
)

type pushLocator struct {
	locator.Locator
	bindings map[string]string
}

func (l *pushLocator) Locate(_ context.Context, _ string, key string) (string, error) {
	return l.bindings[key], nil
}

type pushDiscovery struct {
	registry.Discover
	instances map[string]endpoint.ServiceInstance
}

func (d *pushDiscovery) List(context.Context) (map[string]endpoint.ServiceInstance, error) {
	return d.instances, nil
}

type pushClient struct {
	pbGate.GateClient
	push      func(context.Context, *pbGate.PushRequest) error
	kick      func(context.Context, *pbGate.KickRequest) error
	multi     func(context.Context, *pbGate.MultiPushRequest) (uint32, error)
	broadcast func(context.Context, *pbGate.BroadcastRequest) (uint32, error)
}

func (c *pushClient) Push(ctx context.Context, request *pbGate.PushRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, c.push(ctx, request)
}

func (c *pushClient) Kick(ctx context.Context, request *pbGate.KickRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, c.kick(ctx, request)
}

func (c *pushClient) MultiPush(ctx context.Context, request *pbGate.MultiPushRequest) (*pbGate.MultiPushResponse, error) {
	count, err := c.multi(ctx, request)
	return &pbGate.MultiPushResponse{Count: count}, err
}

func (c *pushClient) Broadcast(ctx context.Context, request *pbGate.BroadcastRequest) (*pbGate.BroadcastResponse, error) {
	count, err := c.broadcast(ctx, request)
	return &pbGate.BroadcastResponse{Count: count}, err
}

func TestPushAndKickLocateGate(t *testing.T) {
	provider := &pushLocator{bindings: map[string]string{
		"1": `{"instance_id":"gate-2","session_id":42}`,
	}}
	client := &pushClient{}
	nodeApp := &Node{
		instance:    endpoint.ServiceInstance{Id: "player-1", ServiceName: "player"},
		gateLocator: locator.NewGateLocator(provider),
		gateClient:  client,
	}
	pushes, kicks := 0, 0
	wantErr := errors.New("send failed")
	client.push = func(ctx context.Context, request *pbGate.PushRequest) error {
		pushes++
		strategy := lx.GetStrategy(ctx)
		if strategy.Mode != lx.ModeNode || strategy.Key != "gate-2" {
			t.Fatalf("push target=%+v", strategy)
		}
		if request.Uid != 1 || request.Route != 100 || string(request.Body) != "body" || request.NodeInstanceId != "player-1" {
			t.Fatalf("push request=%v", request)
		}
		return wantErr
	}
	client.kick = func(ctx context.Context, request *pbGate.KickRequest) error {
		kicks++
		strategy := lx.GetStrategy(ctx)
		if strategy.Mode != lx.ModeNode || strategy.Key != "gate-2" || request.SessionId != 42 {
			t.Fatalf("kick target=%+v request=%v", strategy, request)
		}
		return nil
	}
	ctx := lx.WithNode(context.Background(), "wrong-gate")
	if err := nodeApp.Push(ctx, 1, 100, []byte("body")); err != wantErr {
		t.Fatalf("push error=%v", err)
	}
	if err := nodeApp.Kick(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := nodeApp.Push(ctx, 2, 100, nil); !errors.Is(err, network.ErrNotBound) {
		t.Fatalf("offline push=%v", err)
	}
	if err := nodeApp.Kick(ctx, 2); err != nil {
		t.Fatalf("offline kick=%v", err)
	}
	if pushes != 1 || kicks != 1 {
		t.Fatalf("pushes=%d kicks=%d", pushes, kicks)
	}
}

func TestMultiPushGroupsByGate(t *testing.T) {
	provider := &pushLocator{bindings: map[string]string{
		"1": `{"instance_id":"gate-1","session_id":1}`,
		"2": `{"instance_id":"gate-2","session_id":2}`,
		"3": `{"instance_id":"gate-1","session_id":3}`,
	}}
	client := &pushClient{}
	nodeApp := &Node{gateLocator: locator.NewGateLocator(provider), gateClient: client}
	groups := make(map[string][]uint64)
	client.multi = func(ctx context.Context, request *pbGate.MultiPushRequest) (uint32, error) {
		strategy := lx.GetStrategy(ctx)
		if strategy.Mode != lx.ModeNode || request.Route != 100 || string(request.Body) != "body" {
			t.Fatalf("target=%+v request=%v", strategy, request)
		}
		groups[strategy.Key] = request.UidList
		return uint32(len(request.UidList)), nil
	}
	count, err := nodeApp.MultiPush(context.Background(), []uint64{1, 2, 3, 4}, 100, []byte("body"))
	want := map[string][]uint64{"gate-1": {1, 3}, "gate-2": {2}}
	if err != nil || count != 3 || !reflect.DeepEqual(groups, want) {
		t.Fatalf("count=%d error=%v groups=%v", count, err, groups)
	}

	calls := 0
	wantErr := errors.New("gate failed")
	client.multi = func(context.Context, *pbGate.MultiPushRequest) (uint32, error) {
		calls++
		if calls == 1 {
			return 1, nil
		}
		return 0, wantErr
	}
	count, err = nodeApp.MultiPush(context.Background(), []uint64{1, 2}, 100, nil)
	if count != 1 || err != wantErr || calls != 2 {
		t.Fatalf("partial count=%d error=%v calls=%d", count, err, calls)
	}
}

func TestBroadcastVisitsEveryGate(t *testing.T) {
	client := &pushClient{}
	nodeApp := &Node{
		gateClient: client,
		discovery: &pushDiscovery{instances: map[string]endpoint.ServiceInstance{
			"gate-1":   {Id: "gate-1", ServiceName: locator.GateName},
			"gate-2":   {Id: "gate-2", ServiceName: locator.GateName},
			"player-1": {Id: "player-1", ServiceName: "player"},
		}},
	}
	visited := make(map[string]bool)
	client.broadcast = func(ctx context.Context, request *pbGate.BroadcastRequest) (uint32, error) {
		strategy := lx.GetStrategy(ctx)
		if strategy.Mode != lx.ModeNode || strategy.Key == "player-1" || visited[strategy.Key] {
			t.Fatalf("broadcast target=%+v", strategy)
		}
		visited[strategy.Key] = true
		return 3, nil
	}
	count, err := nodeApp.Broadcast(context.Background(), 100, nil)
	if err != nil || count != 6 || len(visited) != 2 {
		t.Fatalf("count=%d error=%v visited=%v", count, err, visited)
	}
	calls := 0
	wantErr := errors.New("gate failed")
	client.broadcast = func(context.Context, *pbGate.BroadcastRequest) (uint32, error) {
		calls++
		if calls == 1 {
			return 3, nil
		}
		return 0, wantErr
	}
	count, err = nodeApp.Broadcast(context.Background(), 100, nil)
	if count != 3 || err != wantErr || calls != 2 {
		t.Fatalf("partial count=%d error=%v calls=%d", count, err, calls)
	}
}
