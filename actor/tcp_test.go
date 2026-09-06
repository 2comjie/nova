package actor

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	actorSimple "github.com/2comjie/nova/actor/simple"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	"github.com/2comjie/nova/rpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestTCPActorCallsUseMailbox(t *testing.T) {
	server := rpc.NewServer()
	system := NewSystem(server)
	manager := system.Register(1, actorGuard.New("node", &leaseStore{}, actorGuard.WithTTL(time.Hour)),
		func(context.Context, actorDef.Pid) (*actorSimple.SimpleActor, error) {
			return &actorSimple.SimpleActor{}, nil
		}, RunnerConfig{UpdateDt: time.Hour})
	// 故意不加锁；同一 Actor 的并发 RPC 必须由 mailbox 串行执行。
	var count int64
	manager.RPC().Handle(1, func(_ *actorSimple.SimpleActor, _ actorDef.Pid, _ context.Context, _ Message) ([]byte, error) {
		count++
		return proto.Marshal(wrapperspb.Int64(count))
	})
	manager.RPC().Handle(2, func(_ *actorSimple.SimpleActor, _ actorDef.Pid, _ context.Context, _ Message) ([]byte, error) {
		return nil, rpc.NewError(101, "business failure")
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := system.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	conn := rpc.NewConn(listener.Addr().String())
	t.Cleanup(func() { _ = conn.Close() })
	client := pbActor.NewActorClient(conn)
	values := make(chan int64, 64)
	var calls sync.WaitGroup
	for range 64 {
		calls.Go(func() {
			response, err := client.Ask(t.Context(), &pbActor.Request{
				ActorType: 1, ActorKey: "test", Route: 1, Activation: uint32(ActivationLoad),
			})
			if err != nil {
				t.Error(err)
				return
			}
			value := new(wrapperspb.Int64Value)
			if err := proto.Unmarshal(response.Body, value); err != nil {
				t.Error(err)
				return
			}
			if !response.Handled {
				t.Error("actor did not handle request")
			}
			values <- value.Value
		})
	}
	calls.Wait()
	close(values)
	seen := make(map[int64]bool)
	for value := range values {
		if value < 1 || value > 64 || seen[value] {
			t.Errorf("counter=%d", value)
		}
		seen[value] = true
	}
	if len(seen) != 64 {
		t.Fatalf("handled=%d, want 64", len(seen))
	}
	_, err = client.Ask(t.Context(), &pbActor.Request{ActorType: 1, ActorKey: "test", Route: 2, Activation: uint32(ActivationLoad)})
	failure, ok := err.(*rpc.Error)
	if !ok || failure.Code != 101 {
		t.Fatalf("business error=%v", err)
	}
}
