package actor

import (
	"context"
	"testing"

	"github.com/2comjie/nova/actor/actorDef"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	"github.com/2comjie/nova/rpc/rpcerr"
	"google.golang.org/grpc"
)

func TestSystemReturnsActorRedirectCode(t *testing.T) {
	system := NewSystem(grpc.NewServer())
	system.routes[1] = map[uint32]rpcProcessor{
		1001: func(context.Context, actorDef.Key, ActivationPolicy, Message, bool) ([]byte, bool, error) {
			return nil, false, rpcerr.NewWithDetail(ErrorCodeActorRedirect, "actor guarded by instance player-2", []byte("player-2"))
		},
	}

	_, err := system.Ask(context.Background(), &pbActor.Request{ActorType: 1, ActorKey: "uid-1001", Route: 1001})
	redirect, ok := err.(rpcerr.Err)
	if !ok || redirect.Code() != ErrorCodeActorRedirect || string(redirect.Detail()) != "player-2" {
		t.Fatalf("error=%v", err)
	}
}
