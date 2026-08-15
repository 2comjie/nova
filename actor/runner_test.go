package actor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/wali/actor/actorDef"
	actorSimple "github.com/2comjie/wali/actor/simple"
)

func TestRunnerDrainsAcceptedMessagesBeforeUnload(t *testing.T) {
	var processed atomic.Int32
	var persisted atomic.Int32
	firstRunning := make(chan struct{})
	releaseFirst := make(chan struct{})
	actorValue := &actorSimple.SimpleActor{
		MStop: func(actorDef.ActorStopCtx) error {
			persisted.Store(processed.Load())
			return nil
		},
	}
	runner := NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "uid-1001"}, actorValue, RunnerConfig{UpdateDt: time.Hour})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {
		close(firstRunning)
		<-releaseFirst
		processed.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	<-firstRunning
	for range 100 {
		if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {
			processed.Add(1)
		}); err != nil {
			t.Fatal(err)
		}
	}

	runner.RequestStop(actorDef.StopReasonUnload)
	close(releaseFirst)
	<-runner.Done()
	if processed.Load() != 101 {
		t.Fatalf("processed=%d", processed.Load())
	}
	if persisted.Load() != 101 {
		t.Fatalf("persisted=%d", persisted.Load())
	}
}
