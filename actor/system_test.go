package actor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	actorSimple "github.com/2comjie/nova/actor/simple"
	"github.com/2comjie/nova/rpc"
)

func TestSystemRequestStopDoesNotWaitForOnStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	system := NewSystem(rpc.NewServer())
	onStop, allowStop := make(chan struct{}), make(chan struct{})
	manager := system.Register(1, actorGuard.New("node", &leaseStore{}, actorGuard.WithTTL(time.Hour)), func(context.Context, actorDef.Pid) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{MStop: func(actorDef.ActorStopCtx) { close(onStop); <-allowStop }}, nil
	}, RunnerConfig{UpdateDt: time.Hour})
	runner, err := manager.GetOrLoadActor(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	requested := make(chan struct{})
	go func() { system.RequestStop(); close(requested) }()
	select {
	case <-requested:
	case <-ctx.Done():
		t.Fatal("RequestStop waited for OnStop")
	}
	select {
	case <-onStop:
	case <-ctx.Done():
		t.Fatal("RequestStop did not stop the actor")
	}
	system.RequestStop()
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {}); !errors.Is(err, ErrRunnerStopped) {
		t.Fatalf("new task error = %v", err)
	}
	if _, err := manager.GetOrLoadActor(ctx, "another"); !errors.Is(err, ErrSystemStopped) {
		t.Fatalf("new activation error = %v", err)
	}
	canceled, cancelWait := context.WithCancel(ctx)
	cancelWait()
	if err := system.Shutdown(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown before OnStop completed = %v", err)
	}
	close(allowStop)
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
