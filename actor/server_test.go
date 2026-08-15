package actor

import (
	"context"
	"errors"
	"testing"

	"github.com/2comjie/wali/actor/actorDef"
	pbActor "github.com/2comjie/wali/internal/pb/transport/actor"
	"google.golang.org/grpc"
)

func TestServerReturnsActorRedirectCode(t *testing.T) {
	server := NewServer(grpc.NewServer())
	server.routes[1] = map[uint32]rpcProcessor{
		1001: func(context.Context, actorDef.Key, ActivationPolicy, Message, bool) ([]byte, bool, error) {
			return nil, false, ActorRedirectError("player-2")
		},
	}

	_, err := server.Ask(context.Background(), &pbActor.Request{ActorType: 1, ActorKey: "uid-1001", Route: 1001})
	var redirect ActorRedirectError
	if !errors.As(err, &redirect) || redirect.ErrorCode() != ErrorCodeActorRedirect || redirect.RedirectInstanceId() != "player-2" {
		t.Fatalf("error=%v", err)
	}
}
