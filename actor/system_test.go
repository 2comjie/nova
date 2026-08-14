package actor_test

import (
	"context"
	"testing"
	"time"

	"github.com/2comjie/wali/actor"
	"github.com/2comjie/wali/actor/actorDef"
	actorSimple "github.com/2comjie/wali/actor/simple"
	"github.com/2comjie/wali/logx"
)

func TestActorSystem(t *testing.T) {
	actorSystem := actor.NewSystem[*actorSimple.SimpleActor](context.Background(), 1, func(ctx context.Context, pid actorDef.PID) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{
			MStart: func(ctx actorDef.ActorStartCtx) error {
				logx.Infof("start %v", ctx)
				return nil
			},
			MUpdate: func(ctx actorDef.ActorUpdateCtx) time.Duration {
				logx.Infof("update %v", ctx)
				return 0
			},
			MStop: func(ctx actorDef.ActorStopCtx) error {
				logx.Infof("end %v", ctx)
				return nil
			},
		}, nil
	}, actor.RunnerConfig{})

	if _, exists := actorSystem.TryGetActor("not-exists"); exists {
		t.Fatal("不存在的Actor不应该被找到")
	}
}
