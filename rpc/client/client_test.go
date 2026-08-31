package client

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/2comjie/nova/core/endpoint"
	"github.com/2comjie/nova/rpc/lx"
)

type fakeDiscover struct {
	instances map[string]endpoint.ServiceInstance
}

func (f *fakeDiscover) List(context.Context) (map[string]endpoint.ServiceInstance, error) {
	return f.instances, nil
}

func (f *fakeDiscover) Next(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeDiscover) Get(_ context.Context, instanceID string) (endpoint.ServiceInstance, bool, error) {
	instance, ok := f.instances[instanceID]
	return instance, ok, nil
}

func (f *fakeDiscover) Close() {}

type fakeLocator struct {
	instanceID string
	binding    string
}

func (f *fakeLocator) Bind(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeLocator) Unbind(context.Context, string, string, string) error { return nil }
func (f *fakeLocator) Restore(context.Context, string, string, string, string) (bool, error) {
	return true, nil
}
func (f *fakeLocator) Locate(_ context.Context, binding string, _ string) (string, error) {
	f.binding = binding
	return f.instanceID, nil
}
func (f *fakeLocator) Close() {}

func TestPickServiceWeightedRoundRobin(t *testing.T) {
	c := newTestClient(t)
	counts := make(map[string]int)
	for range 40 {
		instance, err := c.pickService(context.Background(), "game", lx.BalanceWeightedRoundRobin)
		if err != nil {
			t.Fatal(err)
		}
		counts[instance.ID]++
	}

	if counts["node-1"] != 10 || counts["node-2"] != 30 {
		t.Fatalf("weighted picks = %v, want node-1:10 node-2:30", counts)
	}
}

func TestPickServiceRoundRobin(t *testing.T) {
	c := newTestClient(t)
	counts := make(map[string]int)
	for range 10 {
		instance, err := c.pickService(context.Background(), "game", lx.BalanceRoundRobin)
		if err != nil {
			t.Fatal(err)
		}
		counts[instance.ID]++
	}

	if counts["node-1"] != 5 || counts["node-2"] != 5 {
		t.Fatalf("round-robin picks = %v, want node-1:5 node-2:5", counts)
	}
}

func TestRouteUsesLocatorInstance(t *testing.T) {
	c := newTestClient(t)
	conn, err := c.Route(context.Background(), "game", "team", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.Target(); got != "127.0.0.1:9002" {
		t.Fatalf("Route target = %q, want %q", got, "127.0.0.1:9002")
	}
	if c.locator.(*fakeLocator).binding != "team" {
		t.Fatalf("Locator binding = %q, want team", c.locator.(*fakeLocator).binding)
	}
}

type fixedBalancer struct {
	instanceID string
}

func (b *fixedBalancer) Pick(
	_ context.Context,
	_ string,
	instances []endpoint.ServiceInstance,
) (endpoint.ServiceInstance, error) {
	for _, instance := range instances {
		if instance.ID == b.instanceID {
			return instance, nil
		}
	}
	return endpoint.ServiceInstance{}, errors.New("instance not found")
}

func TestCustomBalancer(t *testing.T) {
	const policy lx.BalancePolicy = "fixed"
	c := newTestClient(t, WithBalancer(policy, &fixedBalancer{instanceID: "node-2"}))

	ctx := lx.WithBalance(context.Background(), "game", policy)
	conn, err := c.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.Target(); got != "127.0.0.1:9002" {
		t.Fatalf("custom balancer target = %q, want 127.0.0.1:9002", got)
	}
}

func TestUnknownBalancer(t *testing.T) {
	c := newTestClient(t)
	_, err := c.pickService(context.Background(), "game", "unknown")
	if !errors.Is(err, ErrInvalidBalancePolicy) {
		t.Fatalf("pick error = %v, want %v", err, ErrInvalidBalancePolicy)
	}
}

func TestPickActorStableAcrossInstanceOrder(t *testing.T) {
	c := newTestClient(t)
	first, err := c.pickActor("game", "1:uid-1001")
	if err != nil {
		t.Fatal(err)
	}

	c.mu.Lock()
	instances := c.serviceMap["game"]
	c.serviceMap["game"] = []endpoint.ServiceInstance{instances[1], instances[0]}
	c.mu.Unlock()

	second, err := c.pickActor("game", "1:uid-1001")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("actor moved after reorder: %s -> %s", first.ID, second.ID)
	}
}

func TestPickActorMovesSubsetWhenNodeAdded(t *testing.T) {
	c := newTestClient(t)
	before := make(map[string]string, 1000)
	for index := range 1000 {
		key := fmt.Sprintf("1:uid-%d", index)
		instance, err := c.pickActor("game", key)
		if err != nil {
			t.Fatal(err)
		}
		before[key] = instance.ID
	}

	c.update(map[string]endpoint.ServiceInstance{
		"node-1": {ID: "node-1", ServiceName: "game", RpcHost: "127.0.0.1", RpcPort: 9001, Status: endpoint.Working},
		"node-2": {ID: "node-2", ServiceName: "game", RpcHost: "127.0.0.1", RpcPort: 9002, Status: endpoint.Working},
		"node-3": {ID: "node-3", ServiceName: "game", RpcHost: "127.0.0.1", RpcPort: 9003, Status: endpoint.Working},
	})

	moved := 0
	for key, previous := range before {
		instance, err := c.pickActor("game", key)
		if err != nil {
			t.Fatal(err)
		}
		if instance.ID != previous {
			moved++
			if instance.ID != "node-3" {
				t.Fatalf("actor %s moved between existing nodes: %s -> %s", key, previous, instance.ID)
			}
		}
	}
	if moved < 200 || moved > 450 {
		t.Fatalf("moved actors=%d", moved)
	}
}

func TestActorConnection(t *testing.T) {
	c := newTestClient(t)
	instanceId, err := c.ActorInstanceId(context.Background(), "game", "1:uid-1001")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := c.Actor(context.Background(), "game", "1:uid-1001")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"node-1": "127.0.0.1:9001", "node-2": "127.0.0.1:9002"}[instanceId]
	if conn.Target() != want {
		t.Fatalf("target=%s want=%s", conn.Target(), want)
	}
}

func newTestClient(t *testing.T, opts ...Option) *Client {
	t.Helper()
	discover := &fakeDiscover{instances: map[string]endpoint.ServiceInstance{
		"node-1": {
			ID: "node-1", ServiceName: "game", Weight: 1,
			RpcHost: "127.0.0.1", RpcPort: 9001, Status: endpoint.Working,
		},
		"node-2": {
			ID: "node-2", ServiceName: "game", Weight: 3,
			RpcHost: "127.0.0.1", RpcPort: 9002, Status: endpoint.Working,
		},
	}}
	c := NewClient(discover, &fakeLocator{instanceID: "node-2"}, opts...)
	t.Cleanup(c.Close)
	return c
}
