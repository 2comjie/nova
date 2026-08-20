package diff_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type lifecycleActor struct {
	startCount  int
	updateCount int
	stopCount   int
}

func (a *lifecycleActor) OnStart(actorDef.ActorStartCtx) error {
	a.startCount++
	return nil
}

func (a *lifecycleActor) OnUpdate(actorDef.ActorUpdateCtx) time.Duration {
	a.updateCount++
	return 0
}

func (a *lifecycleActor) OnStop(actorDef.ActorStopCtx) error {
	a.stopCount++
	return nil
}

func TestDiffActorPeriodicAndBusinessSave(t *testing.T) {
	value := wrapperspb.Int64(10)
	state := &int64State{value: value}
	realActor := &lifecycleActor{}
	var pushes [][]byte
	var saves [][]byte
	diffActor := diff.NewActor[*lifecycleActor, *wrapperspb.Int64Value](
		logx.WithName("diff-actor-test"),
		realActor,
		func(*diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], actorDef.ActorStartCtx) (diff.SnapState[*wrapperspb.Int64Value], uint64, error) {
			return state, 100, nil
		},
		func(_ *diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], data []byte) {
			pushes = append(pushes, bytes.Clone(data))
		},
		func(_ *diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], data []byte) {
			saves = append(saves, bytes.Clone(data))
		},
		diff.ActorConfig{
			ClientSaveDt: 100 * time.Millisecond,
			ServerSaveDt: time.Hour,
			DiffCount:    8,
		},
	)

	pid := actorDef.PID{Type: 1, Key: "1001"}
	ctx := context.Background()
	if err := diffActor.OnStart(actorDef.ActorStartCtx{Context: ctx, Self: pid}); err != nil {
		t.Fatal(err)
	}
	if realActor.startCount != 1 || diffActor.Version() != 100 || diffActor.Self() != pid {
		t.Fatalf("启动状态错误: start=%d version=%d pid=%v", realActor.startCount, diffActor.Version(), diffActor.Self())
	}

	state.SetValue(20)
	diffActor.OnUpdate(actorDef.ActorUpdateCtx{Context: ctx, Self: pid, Delta: 50 * time.Millisecond})
	if len(pushes) != 0 || len(saves) != 0 || diffActor.Version() != 100 {
		t.Fatalf("未到定时周期时脏数据被自动保存: pushes=%d saves=%d version=%d", len(pushes), len(saves), diffActor.Version())
	}

	diffActor.MarkNeedSave("业务请求推送")
	diffActor.OnUpdate(actorDef.ActorUpdateCtx{Context: ctx, Self: pid, Delta: time.Millisecond})
	if len(pushes) != 1 || len(saves) != 0 || diffActor.Version() != 101 {
		t.Fatalf("客户端保存错误: pushes=%d saves=%d version=%d", len(pushes), len(saves), diffActor.Version())
	}
	clientReader, err := diff.NewSyncReader(pushes[0])
	if err != nil {
		t.Fatal(err)
	}
	if clientReader.BaseVersion() != 100 || clientReader.Version() != 101 {
		t.Fatalf("客户端增量版本错误: %d->%d", clientReader.BaseVersion(), clientReader.Version())
	}

	state.SetValue(30)
	diffActor.OnUpdate(actorDef.ActorUpdateCtx{Context: ctx, Self: pid, Delta: 99 * time.Millisecond})
	if len(pushes) != 1 || diffActor.Version() != 101 {
		t.Fatalf("未到定时周期时发生推送: pushes=%d version=%d", len(pushes), diffActor.Version())
	}
	diffActor.OnUpdate(actorDef.ActorUpdateCtx{Context: ctx, Self: pid, Delta: time.Millisecond})
	if len(pushes) != 2 || diffActor.Version() != 102 {
		t.Fatalf("定时推送错误: pushes=%d version=%d", len(pushes), diffActor.Version())
	}

	state.SetValue(40)
	diffActor.ForceSave("业务要求客户端和服务端保存")
	diffActor.OnUpdate(actorDef.ActorUpdateCtx{Context: ctx, Self: pid, Delta: time.Millisecond})
	if len(pushes) != 3 || len(saves) != 1 || diffActor.Version() != 103 {
		t.Fatalf("强制保存错误: pushes=%d saves=%d version=%d", len(pushes), len(saves), diffActor.Version())
	}
	serverReader, err := diff.NewSyncReader(saves[0])
	if err != nil {
		t.Fatal(err)
	}
	if serverReader.BaseVersion() != 100 || serverReader.Version() != 103 {
		t.Fatalf("服务端增量版本错误: %d->%d", serverReader.BaseVersion(), serverReader.Version())
	}
	for count := 0; ; count++ {
		_, ok, err := serverReader.NextDiff()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			if count != 3 {
				t.Fatalf("服务端增量数量错误: %d", count)
			}
			break
		}
	}
	if diffActor.ClientNeedSave() || diffActor.ServerNeedSave() {
		t.Fatalf("保存标记没有清理: client=%t server=%t", diffActor.ClientNeedSave(), diffActor.ServerNeedSave())
	}
}

func TestDiffActorSavesPendingServerDataOnStop(t *testing.T) {
	state := &int64State{value: wrapperspb.Int64(10)}
	realActor := &lifecycleActor{}
	var saves [][]byte
	diffActor := diff.NewActor[*lifecycleActor, *wrapperspb.Int64Value](
		logx.WithName("diff-actor-stop-test"),
		realActor,
		func(*diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], actorDef.ActorStartCtx) (diff.SnapState[*wrapperspb.Int64Value], uint64, error) {
			return state, 200, nil
		},
		func(*diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], []byte) {},
		func(_ *diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], data []byte) {
			saves = append(saves, bytes.Clone(data))
		},
		diff.ActorConfig{},
	)

	pid := actorDef.PID{Type: 1, Key: "1002"}
	ctx := context.Background()
	if err := diffActor.OnStart(actorDef.ActorStartCtx{Context: ctx, Self: pid}); err != nil {
		t.Fatal(err)
	}
	state.SetValue(20)
	diffActor.ForceSave()
	if err := diffActor.OnStop(actorDef.ActorStopCtx{Context: ctx, Self: pid, Reason: actorDef.StopReasonShutdown}); err != nil {
		t.Fatal(err)
	}
	if realActor.stopCount != 1 || len(saves) != 1 || diffActor.Version() != 201 {
		t.Fatalf("停止保存错误: stop=%d saves=%d version=%d", realActor.stopCount, len(saves), diffActor.Version())
	}
}

func TestDiffActorDoesNotSaveAfterLeaseLost(t *testing.T) {
	state := &int64State{value: wrapperspb.Int64(10)}
	var saveCount int
	diffActor := diff.NewActor[*lifecycleActor, *wrapperspb.Int64Value](
		logx.WithName("diff-actor-lease-test"),
		&lifecycleActor{},
		func(*diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], actorDef.ActorStartCtx) (diff.SnapState[*wrapperspb.Int64Value], uint64, error) {
			return state, 300, nil
		},
		func(*diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], []byte) {},
		func(*diff.Actor[*lifecycleActor, *wrapperspb.Int64Value], []byte) {
			saveCount++
		},
		diff.ActorConfig{},
	)

	pid := actorDef.PID{Type: 1, Key: "1003"}
	ctx := context.Background()
	if err := diffActor.OnStart(actorDef.ActorStartCtx{Context: ctx, Self: pid}); err != nil {
		t.Fatal(err)
	}
	state.SetValue(20)
	diffActor.ForceSave()
	if err := diffActor.OnStop(actorDef.ActorStopCtx{Context: ctx, Self: pid, Reason: actorDef.StopReasonLeaseLost}); err != nil {
		t.Fatal(err)
	}
	if saveCount != 0 || diffActor.Version() != 300 {
		t.Fatalf("丢失租约后仍然保存: saves=%d version=%d", saveCount, diffActor.Version())
	}
}
