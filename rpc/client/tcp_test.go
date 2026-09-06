package client

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/2comjie/nova/core/endpoint"
	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	novaRPC "github.com/2comjie/nova/rpc"
	"github.com/2comjie/nova/rpc/lx"
	"github.com/spf13/cast"
)

type testNodeServer struct {
	pbNode.UnimplementedNodeServer
	started chan struct{}
	release chan struct{}
}

func (s *testNodeServer) Call(ctx context.Context, request *pbNode.Request) (*pbNode.Response, *novaRPC.Error) {
	switch request.Route {
	case 1:
		return nil, novaRPC.NewErrorWithDetail(42, "not enough coins", []byte("details"))
	case 2:
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, novaRPC.FromError(ctx.Err())
		}
	}
	return &pbNode.Response{Replied: true}, nil
}

func TestTCPClient(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := novaRPC.NewServer()
	service := &testNodeServer{started: make(chan struct{}), release: make(chan struct{})}
	pbNode.RegisterNodeServer(server, service)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil && err != novaRPC.ErrClosed {
			t.Error(err)
		}
	})
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	discover := &fakeDiscover{instances: map[string]endpoint.ServiceInstance{
		"game-1": {Id: "game-1", ServiceName: "game", RpcHost: host, RpcPort: cast.ToInt(port), Status: endpoint.Working},
	}}
	client := NewClient(discover, nil)
	t.Cleanup(client.Close)
	typed := pbNode.NewNodeClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	routed := lx.WithBalance(ctx, "game")
	if response, err := typed.Call(routed, &pbNode.Request{}); err != nil || !response.Replied {
		t.Fatalf("response=%v err=%v", response, err)
	}
	_, err = typed.Call(routed, &pbNode.Request{Route: 1})
	var failure *novaRPC.Error
	if !errors.As(err, &failure) || failure.Code != 42 || string(failure.Detail) != "details" {
		t.Fatalf("business error=%v", err)
	}
	if _, err := typed.Call(ctx, &pbNode.Request{}); err != ErrInvalidTarget {
		t.Fatalf("unrouted error=%v", err)
	}
	result := make(chan error, 1)
	go func() { _, err := typed.Call(routed, &pbNode.Request{Route: 2}); result <- err }()
	select {
	case <-service.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	client.update(nil)
	if _, err := typed.Call(routed, &pbNode.Request{}); err != ErrNoAnyService {
		t.Fatalf("removed node error=%v", err)
	}
	close(service.release)
	if err := <-result; err != nil {
		t.Fatalf("in-flight call=%v", err)
	}
	client.Close()
	if _, err := client.Direct(ctx, listener.Addr().String()); err != ErrClosed {
		t.Fatalf("closed error=%v", err)
	}
}

func TestConnPoolCloseDuringGet(t *testing.T) {
	pool := NewConnPool()
	var calls sync.WaitGroup
	for range 32 {
		calls.Go(func() { _, _ = pool.Get("127.0.0.1:9001") })
	}
	pool.Close()
	calls.Wait()
	if _, err := pool.Get("127.0.0.1:9001"); err != ErrClosed {
		t.Fatalf("Get after Close=%v", err)
	}
	if pool.connMap != nil {
		t.Fatal("connection installed after Close")
	}
}
