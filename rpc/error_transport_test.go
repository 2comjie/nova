package rpc_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/2comjie/wali/core/endpoint"
	pbActor "github.com/2comjie/wali/internal/pb/transport/actor"
	"github.com/2comjie/wali/rpc"
	rpcClient "github.com/2comjie/wali/rpc/client"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type errorServer struct {
	pbActor.UnimplementedActorServer
}

func (errorServer) Ask(context.Context, *pbActor.Request) (*pbActor.Response, error) {
	return nil, rpc.NewErrorWithDetail(10001, "coin not enough", []byte("player-2"))
}

func (errorServer) Tell(context.Context, *pbActor.Request) (*pbActor.Response, error) {
	return nil, status.Error(codes.Unavailable, "service unavailable")
}

type emptyDiscover struct{}

func (emptyDiscover) List(context.Context) (map[string]endpoint.ServiceInstance, error) {
	return nil, nil
}

func (emptyDiscover) Next(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (emptyDiscover) Get(context.Context, string) (endpoint.ServiceInstance, bool, error) {
	return endpoint.ServiceInstance{}, false, nil
}

func (emptyDiscover) Close() {}

func TestGeneratedRPCErrorTransport(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	pbActor.RegisterActorServer(grpcServer, errorServer{})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	client := rpcClient.NewClient(emptyDiscover{}, nil, rpcClient.WithDialOptions(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	})))
	t.Cleanup(client.Close)
	actorClient := pbActor.NewActorClient(client)
	ctx := lx.WithDirect(context.Background(), "127.0.0.1:1")

	_, err := actorClient.Ask(ctx, &pbActor.Request{})
	var rpcError *rpc.Error
	if !errors.As(err, &rpcError) {
		t.Fatalf("ask error=%v", err)
	}
	if rpcError.Code != 10001 || rpcError.Message != "coin not enough" || string(rpcError.Detail) != "player-2" {
		t.Fatalf("rpc error=%+v", rpcError)
	}

	_, err = actorClient.Tell(ctx, &pbActor.Request{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("tell error=%v", err)
	}
}
