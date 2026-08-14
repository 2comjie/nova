package actor_test

import (
	"context"
	"testing"
	"time"

	"github.com/2comjie/wali/actor"
	"github.com/2comjie/wali/actor/actorDef"
	actorSimple "github.com/2comjie/wali/actor/simple"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/logx"
)

func TestRunner(t *testing.T) {
	testActor := &actorSimple.SimpleActor{
		MUpdate: func(ctx actorDef.ActorUpdateCtx) time.Duration {
			logx.Infof("update %+v", ctx)
			return 0
		},
		MStart: func(ctx actorDef.ActorStartCtx) error {
			logx.Infof("start %+v", ctx)
			return nil
		},
		MStop: func(ctx actorDef.ActorStopCtx) error {
			logx.Infof("stop %+v", ctx)
			return nil
		},
	}

	runner := actor.NewRunner(context.Background(), actorDef.PID{
		Type: 1,
		Key:  "test-1001",
	}, testActor, actor.RunnerConfig{
		QueueCap: 100,
		UpdateDt: 2 * time.Second,
	})
	err := runner.Start()
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		tk := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-done:
				return
			case <-tk.C:
				_ = runner.RunOnMainLoop(func(a *actorSimple.SimpleActor) {
					logx.Infof("hello")
				})
			}
		}
	}()
	util.WaitUntilSignaled()
	done <- struct{}{}
	err = runner.Stop(actorDef.StopReasonShutdown)
	<-runner.Done()
}
