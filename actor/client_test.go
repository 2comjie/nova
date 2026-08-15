package actor

import (
	"context"
	"errors"
	"testing"

	"github.com/2comjie/wali/actor/actorDef"
	pbActor "github.com/2comjie/wali/internal/pb/transport/actor"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
)

type clientCall struct {
	method   string
	strategy lx.Strategy
}

type fakeActorClient struct {
	calls          []clientCall
	secondRedirect bool
	errorCode      uint32
}

func (c *fakeActorClient) Ask(ctx context.Context, _ *pbActor.Request, _ ...grpc.CallOption) (*pbActor.Response, error) {
	c.calls = append(c.calls, clientCall{method: "ask", strategy: lx.GetStrategy(ctx)})
	if len(c.calls) == 1 || c.secondRedirect {
		return &pbActor.Response{RedirectInstanceId: "player-2"}, nil
	}
	if c.errorCode != 0 {
		return &pbActor.Response{ErrorCode: c.errorCode, ErrorMessage: "coin not enough"}, nil
	}
	return &pbActor.Response{Handled: true, Body: []byte{7}}, nil
}

func (c *fakeActorClient) Tell(ctx context.Context, _ *pbActor.Request, _ ...grpc.CallOption) (*pbActor.Response, error) {
	c.calls = append(c.calls, clientCall{method: "tell", strategy: lx.GetStrategy(ctx)})
	if len(c.calls) == 1 || c.secondRedirect {
		return &pbActor.Response{RedirectInstanceId: "player-2"}, nil
	}
	return &pbActor.Response{}, nil
}

func TestRefAskRetriesOwner(t *testing.T) {
	rpc := &fakeActorClient{}
	ref := Ref{
		client:  &Client{rpc: rpc},
		service: "player",
		pid:     actorDef.PID{Type: 1, Key: "uid-1001"},
		policy:  ActivationLoad,
	}

	body, handled, err := ref.Ask(context.Background(), Message{Route: 1001, Body: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(body) != 1 || body[0] != 7 {
		t.Fatalf("handled=%t body=%v", handled, body)
	}
	assertActorRetry(t, rpc.calls)
}

func TestRefTellRetriesOwner(t *testing.T) {
	rpc := &fakeActorClient{}
	ref := Ref{
		client:  &Client{rpc: rpc},
		service: "player",
		pid:     actorDef.PID{Type: 1, Key: "uid-1001"},
		policy:  ActivationLoad,
	}

	if err := ref.Tell(context.Background(), Message{Route: 1001, Body: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	assertActorRetry(t, rpc.calls)
}

func TestRefStopsAfterSecondRedirect(t *testing.T) {
	rpc := &fakeActorClient{secondRedirect: true}
	ref := Ref{
		client:  &Client{rpc: rpc},
		service: "player",
		pid:     actorDef.PID{Type: 1, Key: "uid-1001"},
		policy:  ActivationLoad,
	}

	_, _, err := ref.Ask(context.Background(), Message{Route: 1001})
	if !errors.Is(err, ErrActorGuarded) {
		t.Fatalf("ask error=%v", err)
	}
	assertActorRetry(t, rpc.calls)
}

func TestRefReturnsBusinessError(t *testing.T) {
	rpc := &fakeActorClient{errorCode: 10001}
	ref := Ref{
		client:  &Client{rpc: rpc},
		service: "player",
		pid:     actorDef.PID{Type: 1, Key: "uid-1001"},
		policy:  ActivationLoad,
	}

	_, _, err := ref.Ask(context.Background(), Message{Route: 1001})
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Code != 10001 || callErr.Message != "coin not enough" {
		t.Fatalf("ask error=%v", err)
	}
}

func assertActorRetry(t *testing.T, calls []clientCall) {
	t.Helper()
	if len(calls) != 2 {
		t.Fatalf("calls=%d", len(calls))
	}
	first := calls[0].strategy
	if first.Mode != lx.ModeActor || first.Service != "player" || first.Key != "uid-1001" {
		t.Fatalf("first strategy=%+v", first)
	}
	second := calls[1].strategy
	if second.Mode != lx.ModeNode || second.Key != "player-2" {
		t.Fatalf("second strategy=%+v", second)
	}
}
