package actor_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/wali/actor"
	"github.com/2comjie/wali/actor/actorDef"
	actorSimple "github.com/2comjie/wali/actor/simple"
)

func TestRunnerLifecycle(t *testing.T) {
	pid := actorDef.PID{Type: 1, Key: "test-1001"}
	started := make(chan actorDef.ActorStartCtx, 1)
	stopped := make(chan actorDef.ActorStopCtx, 1)
	testActor := &actorSimple.SimpleActor{
		MStart: func(ctx actorDef.ActorStartCtx) error {
			started <- ctx
			return nil
		},
		MUpdate: func(actorDef.ActorUpdateCtx) time.Duration {
			return -1
		},
		MStop: func(ctx actorDef.ActorStopCtx) error {
			stopped <- ctx
			return nil
		},
	}
	runner := actor.NewRunner(context.Background(), pid, testActor, actor.RunnerConfig{})

	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	if ctx := <-started; ctx.Self != pid || ctx.Err() != nil {
		t.Fatalf("start ctx=%+v", ctx)
	}

	value := 0
	if err := runner.WaitResultOnMainLoop(context.Background(), func(*actorSimple.SimpleActor) {
		value = 100
	}); err != nil {
		t.Fatal(err)
	}
	if value != 100 {
		t.Fatalf("value=%d", value)
	}

	if err := runner.Stop(actorDef.StopReasonUnload); err != nil {
		t.Fatal(err)
	}
	stopCtx := <-stopped
	if stopCtx.Self != pid || stopCtx.Reason != actorDef.StopReasonUnload || stopCtx.Err() != nil {
		t.Fatalf("stop ctx=%+v", stopCtx)
	}
	select {
	case <-runner.Done():
	default:
		t.Fatal("runner done没有关闭")
	}
}

func TestRunnerSerializesTasks(t *testing.T) {
	testActor := &actorSimple.SimpleActor{
		MUpdate: func(actorDef.ActorUpdateCtx) time.Duration {
			return -1
		},
	}
	runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "serial"}, testActor, actor.RunnerConfig{})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop(actorDef.StopReasonShutdown)

	var value int
	errs := make(chan error, 64)
	var wait sync.WaitGroup
	wait.Add(64)
	for range 64 {
		go func() {
			defer wait.Done()
			errs <- runner.WaitResultOnMainLoop(context.Background(), func(*actorSimple.SimpleActor) {
				value++
			})
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if value != 64 {
		t.Fatalf("value=%d", value)
	}
}

func TestRunnerQueueFull(t *testing.T) {
	testActor := &actorSimple.SimpleActor{}
	runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "queue"}, testActor, actor.RunnerConfig{QueueCap: 1, UpdateDt: time.Hour})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {
		close(entered)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	<-entered

	secondErr := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {})
	fullErr := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {})
	close(release)
	if err := runner.Stop(actorDef.StopReasonShutdown); err != nil {
		t.Fatal(err)
	}
	if secondErr != nil {
		t.Fatalf("第二个任务入队失败: %v", secondErr)
	}
	if fullErr == nil || fullErr.Error() != "queue full" {
		t.Fatalf("队列满错误=%v", fullErr)
	}
}

func TestRunnerWaitContextCanceled(t *testing.T) {
	testActor := &actorSimple.SimpleActor{}
	runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "cancel"}, testActor, actor.RunnerConfig{UpdateDt: time.Hour})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop(actorDef.StopReasonShutdown)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var executed atomic.Bool
	err := runner.WaitResultOnMainLoop(ctx, func(*actorSimple.SimpleActor) {
		executed.Store(true)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("等待错误=%v", err)
	}
	if err := runner.WaitResultOnMainLoop(context.Background(), func(*actorSimple.SimpleActor) {}); err != nil {
		t.Fatal(err)
	}
	if executed.Load() {
		t.Fatal("已取消的任务仍然执行")
	}
}

func TestRunnerUpdatePauseAndWake(t *testing.T) {
	updates := make(chan struct{}, 2)
	testActor := &actorSimple.SimpleActor{
		MUpdate: func(actorDef.ActorUpdateCtx) time.Duration {
			updates <- struct{}{}
			return -1
		},
	}
	runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "update"}, testActor, actor.RunnerConfig{UpdateDt: 10 * time.Millisecond})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop(actorDef.StopReasonShutdown)

	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("第一次Update没有执行")
	}
	select {
	case <-updates:
		t.Fatal("暂停后仍然执行Update")
	case <-time.After(50 * time.Millisecond):
	}
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) {}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("新任务没有唤醒Update")
	}
}

func TestRunnerTaskPanicDoesNotBlockOrStop(t *testing.T) {
	testActor := &actorSimple.SimpleActor{}
	runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "task-panic"}, testActor, actor.RunnerConfig{UpdateDt: time.Hour})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop(actorDef.StopReasonShutdown)

	if err := runner.WaitResultOnMainLoop(context.Background(), func(*actorSimple.SimpleActor) {
		panic("task panic")
	}); err != nil {
		t.Fatal(err)
	}
	if err := runner.WaitResultOnMainLoop(context.Background(), func(*actorSimple.SimpleActor) {}); err != nil {
		t.Fatalf("panic后Runner不能继续执行: %v", err)
	}
}

func TestRunnerStartAndStopPanic(t *testing.T) {
	startPanicActor := &actorSimple.SimpleActor{
		MStart: func(actorDef.ActorStartCtx) error {
			panic("start panic")
		},
	}
	startPanicRunner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "start-panic"}, startPanicActor, actor.RunnerConfig{})
	if err := startPanicRunner.Start(); err == nil || err.Error() != "actor start panic" {
		t.Fatalf("OnStart panic错误=%v", err)
	}
	select {
	case <-startPanicRunner.Done():
	case <-time.After(time.Second):
		t.Fatal("OnStart panic后Runner没有停止")
	}

	stopPanicActor := &actorSimple.SimpleActor{
		MStop: func(actorDef.ActorStopCtx) error {
			panic("stop panic")
		},
	}
	stopPanicRunner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "stop-panic"}, stopPanicActor, actor.RunnerConfig{})
	if err := stopPanicRunner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := stopPanicRunner.Stop(actorDef.StopReasonShutdown); err == nil || err.Error() != "actor stop panic" {
		t.Fatalf("OnStop panic错误=%v", err)
	}
}

func TestRunnerStopBeforeStartAndConcurrentStop(t *testing.T) {
	notStarted := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "not-started"}, &actorSimple.SimpleActor{}, actor.RunnerConfig{})
	if err := notStarted.Stop(actorDef.StopReasonShutdown); err != nil {
		t.Fatal(err)
	}
	select {
	case <-notStarted.Done():
	default:
		t.Fatal("未启动Runner停止后done没有关闭")
	}
	if err := notStarted.Start(); err == nil || err.Error() != "already stopped" {
		t.Fatalf("停止后Start错误=%v", err)
	}

	var stopCount atomic.Int32
	runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "concurrent-stop"}, &actorSimple.SimpleActor{
		MStop: func(actorDef.ActorStopCtx) error {
			stopCount.Add(1)
			return nil
		},
	}, actor.RunnerConfig{})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 16)
	var wait sync.WaitGroup
	wait.Add(16)
	for range 16 {
		go func() {
			defer wait.Done()
			errs <- runner.Stop(actorDef.StopReasonShutdown)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if stopCount.Load() != 1 {
		t.Fatalf("OnStop执行次数=%d", stopCount.Load())
	}
}

func TestRunnerActorCanUnloadItself(t *testing.T) {
	t.Run("StartCtx", func(t *testing.T) {
		var unload func()
		stopReason := make(chan actorDef.StopReason, 1)
		testActor := &actorSimple.SimpleActor{
			MStart: func(ctx actorDef.ActorStartCtx) error {
				unload = ctx.Unload
				return nil
			},
			MStop: func(ctx actorDef.ActorStopCtx) error {
				stopReason <- ctx.Reason
				return nil
			},
		}
		runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "self-unload"}, testActor, actor.RunnerConfig{UpdateDt: time.Hour})
		if err := runner.Start(); err != nil {
			t.Fatal(err)
		}
		if err := runner.WaitResultOnMainLoop(context.Background(), func(*actorSimple.SimpleActor) {
			unload()
		}); err != nil {
			t.Fatalf("请求卸载的当前任务执行失败: %v", err)
		}
		select {
		case <-runner.Done():
		case <-time.After(time.Second):
			t.Fatal("Actor主动卸载后Runner没有停止")
		}
		if reason := <-stopReason; reason != actorDef.StopReasonUnload {
			t.Fatalf("停止原因=%d", reason)
		}
	})

	t.Run("UpdateCtx", func(t *testing.T) {
		stopReason := make(chan actorDef.StopReason, 1)
		testActor := &actorSimple.SimpleActor{
			MUpdate: func(ctx actorDef.ActorUpdateCtx) time.Duration {
				ctx.Unload()
				return -1
			},
			MStop: func(ctx actorDef.ActorStopCtx) error {
				stopReason <- ctx.Reason
				return nil
			},
		}
		runner := actor.NewRunner(context.Background(), actorDef.PID{Type: 1, Key: "update-unload"}, testActor, actor.RunnerConfig{UpdateDt: time.Millisecond})
		if err := runner.Start(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-runner.Done():
		case <-time.After(time.Second):
			t.Fatal("Update请求卸载后Runner没有停止")
		}
		if reason := <-stopReason; reason != actorDef.StopReasonUnload {
			t.Fatalf("停止原因=%d", reason)
		}
	})
}
