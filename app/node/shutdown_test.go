package node

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/nova/app"
	"github.com/2comjie/nova/core/endpoint"
	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/registry"
	"github.com/2comjie/nova/rpc"
)

type shutdownRegistry struct{ registry.Registry }

func (*shutdownRegistry) Register(endpoint.ServiceInstance) error { return nil }
func (*shutdownRegistry) Deregister(string) error                 { return nil }

func TestShutdownNotifiesBeforeWaitingForRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rpcReturned := make(chan struct{})
	server := rpc.NewServer()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	entered := make(chan struct{})
	stop := make(chan struct{})
	dependencyStopped := make(chan struct{})
	var notifications atomic.Int32
	dependency := &app.CommonComponent{MShutdown: func(context.Context) error {
		select {
		case <-rpcReturned:
		default:
			t.Error("dependency stopped before the RPC returned")
		}
		close(dependencyStopped)
		return nil
	}}
	worker := &app.CommonComponent{MRequestStop: func() {
		notifications.Add(1)
		close(stop)
	}}
	router := NewRouter()
	router.Handle(1, func(ctx *Context) error {
		defer close(rpcReturned)
		close(entered)
		select {
		case <-stop:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-dependencyStopped:
			t.Error("dependency unavailable while handling RPC")
		default:
		}
		return ctx.Reply([]byte("stopped"))
	})
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	nodeApp := &Node{
		App: app.New(dependency, worker), router: router, registry: &shutdownRegistry{},
		rpcServer: server, rpcListener: listener, ctx: runCtx, cancel: cancel,
	}
	pbNode.RegisterNodeServer(server, nodeApp)
	if err := nodeApp.Start(); err != nil {
		t.Fatal(err)
	}
	conn := rpc.NewConn(listener.Addr().String())
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancelCall := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCall()
	rpcDone := make(chan error, 1)
	go func() {
		response, err := pbNode.NewNodeClient(conn).Call(ctx, &pbNode.Request{
			Uid: 1, Route: 1, GateServiceName: locator.GateName, GateInstanceId: "gate-1",
		})
		if err == nil && (!response.Replied || string(response.Body) != "stopped") {
			t.Errorf("response=%v", response)
		}
		rpcDone <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := nodeApp.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-rpcDone; err != nil {
		t.Fatalf("in-flight RPC interrupted: %v", err)
	}
	select {
	case <-dependencyStopped:
	default:
		t.Fatal("dependency was not stopped")
	}
	if err := nodeApp.Shutdown(ctx); err != nil || notifications.Load() != 1 {
		t.Fatalf("repeat shutdown error=%v notifications=%d", err, notifications.Load())
	}
}
