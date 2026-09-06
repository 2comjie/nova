package actor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	actorSimple "github.com/2comjie/nova/actor/simple"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	"github.com/2comjie/nova/rpc"
	"github.com/redis/go-redis/v9"
)

// These tests exercise lifecycle ordering, not Redis's Lua interpreter.
type leaseStore struct {
	redis.UniversalClient
	mu         sync.Mutex
	owner      string
	releases   int
	acquireSHA string
}

func (s *leaseStore) EvalSha(ctx context.Context, sha string, _ []string, args ...any) *redis.Cmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd := redis.NewCmd(ctx)
	if err := ctx.Err(); err != nil {
		cmd.SetErr(err)
		return cmd
	}
	if len(args) == 1 {
		if s.owner == args[0] {
			s.owner = ""
			s.releases++
			cmd.SetVal(int64(1))
		} else {
			cmd.SetVal(int64(0))
		}
	} else {
		if s.acquireSHA == "" {
			s.acquireSHA = sha
		}
		if sha == s.acquireSHA {
			if s.owner == "" {
				s.owner = args[0].(string)
			}
			cmd.SetVal(s.owner)
		} else if s.owner == args[0] {
			cmd.SetVal(int64(1))
		} else {
			cmd.SetVal(int64(0))
		}
	}
	return cmd
}

func TestRunnerSkipsCanceledPendingAsk(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runner := NewRunner(context.Background(), actorDef.Pid{Type: 1, Key: "test"}, &actorSimple.SimpleActor{}, RunnerConfig{UpdateDt: time.Hour})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) { close(entered); <-resume }); err != nil {
		t.Fatal(err)
	}
	<-entered
	requestCtx, cancelRequest := context.WithCancel(ctx)
	result := make(chan error, 1)
	executed := false
	go func() {
		result <- runner.WaitResultOnMainLoop(requestCtx, func(context.Context, *actorSimple.SimpleActor) error { executed = true; return nil })
	}()
	for len(runner.queue) == 0 {
		if ctx.Err() != nil {
			t.Fatal("Ask was not enqueued")
		}
		runtime.Gosched()
	}
	cancelRequest()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask error = %v", err)
	}
	close(resume)
	runner.Stop(actorDef.StopReasonShutdown)
	if executed {
		t.Fatal("canceled pending Ask executed")
	}
}

func TestLeaseLossOverridesGracefulDrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopped := make(chan actorDef.ActorStopCtx, 1)
	runner := NewRunner(context.Background(), actorDef.Pid{Type: 1, Key: "test"}, &actorSimple.SimpleActor{
		MStop: func(ctx actorDef.ActorStopCtx) { stopped <- ctx },
	}, RunnerConfig{UpdateDt: time.Hour})
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan context.Context, 1)
	result := make(chan error, 1)
	go func() {
		result <- runner.WaitResultOnMainLoop(ctx, func(execCtx context.Context, _ *actorSimple.SimpleActor) error {
			entered <- execCtx
			<-execCtx.Done()
			return execCtx.Err()
		})
	}()
	execCtx := <-entered
	queuedRan := false
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) { queuedRan = true }); err != nil {
		t.Fatal(err)
	}
	runner.RequestStop(actorDef.StopReasonShutdown)
	if execCtx.Err() != nil {
		t.Fatal("graceful drain canceled an accepted task")
	}
	runner.RequestStop(actorDef.StopReasonLeaseLost)
	select {
	case <-runner.Done():
	case <-ctx.Done():
		t.Fatal("lease loss did not stop execution")
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask error = %v", err)
	}
	if queuedRan {
		t.Fatal("lease loss executed a queued task")
	}
	stopCtx := <-stopped
	if stopCtx.Reason != actorDef.StopReasonLeaseLost || stopCtx.Err() == nil {
		t.Fatalf("stop context = %+v", stopCtx)
	}
}

func TestSystemShutdownCancelsOnStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store := &leaseStore{}
	system := NewSystem(rpc.NewServer())
	entered := make(chan struct{})
	manager := system.Register(1, actorGuard.New("node", store, actorGuard.WithTTL(time.Hour)), func(context.Context, actorDef.Pid) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{MStart: func(ctx actorDef.ActorStartCtx) error { close(entered); <-ctx.Done(); return ctx.Err() }}, nil
	}, RunnerConfig{UpdateDt: time.Hour})
	result := make(chan error, 1)
	go func() { _, err := manager.GetOrLoadActor(ctx, "test"); result <- err }()
	<-entered
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v", err)
	}
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.owner != "" || store.releases != 1 {
		t.Fatalf("owner=%q releases=%d", store.owner, store.releases)
	}
}

func TestLeaseLossCancelsLoader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store := &leaseStore{}
	system := NewSystem(rpc.NewServer())
	entered := make(chan struct{})
	manager := system.Register(1, actorGuard.New("node", store, actorGuard.WithTTL(30*time.Millisecond)), func(ctx context.Context, _ actorDef.Pid) (*actorSimple.SimpleActor, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}, RunnerConfig{UpdateDt: time.Hour})
	result := make(chan error, 1)
	go func() { _, err := manager.GetOrLoadActor(ctx, "test"); result <- err }()
	<-entered
	store.mu.Lock()
	store.owner = "new-node"
	store.mu.Unlock()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loader error = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("lease loss did not cancel Loader")
	}
	if _, exists := manager.TryGetActor("test"); exists {
		t.Fatal("lost activation was published")
	}
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUnloadReleasesAfterOnStopAndAllowsReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store := &leaseStore{}
	system := NewSystem(rpc.NewServer())
	onStop, allowStop := make(chan struct{}), make(chan struct{})
	loads := 0
	manager := system.Register(1, actorGuard.New("node", store, actorGuard.WithTTL(time.Hour)), func(context.Context, actorDef.Pid) (*actorSimple.SimpleActor, error) {
		loads++
		value := &actorSimple.SimpleActor{}
		if loads == 1 {
			value.MStop = func(ctx actorDef.ActorStopCtx) {
				if ctx.Err() != nil {
					panic("unload canceled OnStop")
				}
				close(onStop)
				<-allowStop
			}
		}
		return value, nil
	}, RunnerConfig{UpdateDt: time.Hour})
	first, err := manager.GetOrLoadActor(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	unloaded := make(chan struct{})
	go func() { manager.UnloadActor("test"); close(unloaded) }()
	<-onStop
	store.mu.Lock()
	owner, releases := store.owner, store.releases
	store.mu.Unlock()
	if owner != "node" || releases != 0 {
		t.Fatalf("lease released during OnStop: owner=%q releases=%d", owner, releases)
	}
	close(allowStop)
	select {
	case <-unloaded:
	case <-ctx.Done():
		t.Fatal("unload did not complete")
	}
	second, err := manager.GetOrLoadActor(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || loads != 2 {
		t.Fatal("reload reused the stopped actor")
	}
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTellReturnsAfterEnqueueAndUsesActorContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	system := NewSystem(rpc.NewServer())
	manager := system.Register(1, actorGuard.New("node", &leaseStore{}, actorGuard.WithTTL(time.Hour)), func(context.Context, actorDef.Pid) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{}, nil
	}, RunnerConfig{UpdateDt: time.Hour})
	executed := make(chan error, 1)
	manager.RPC().Handle(1, func(_ *actorSimple.SimpleActor, _ actorDef.Pid, execCtx context.Context, _ Message) ([]byte, error) {
		executed <- execCtx.Err()
		return nil, nil
	})
	runner, err := manager.GetOrLoadActor(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) { close(entered); <-resume }); err != nil {
		t.Fatal(err)
	}
	<-entered
	requestCtx, cancelRequest := context.WithCancel(ctx)
	response, rpcErr := system.Tell(requestCtx, &pbActor.Request{ActorType: 1, ActorKey: "test", Route: 1, Activation: uint32(ActivationLoad)})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if !response.Handled {
		t.Fatal("Tell was not accepted")
	}
	cancelRequest()
	close(resume)
	select {
	case err := <-executed:
		if err != nil {
			t.Fatalf("Tell inherited its canceled request context: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Tell did not execute")
	}
	if err := system.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestActorPanicExitsProcess(t *testing.T) {
	if os.Getenv("NOVA_ACTOR_PANIC_TEST") == "1" {
		runner := NewRunner(context.Background(), actorDef.Pid{Type: 1, Key: "test"}, &actorSimple.SimpleActor{}, RunnerConfig{UpdateDt: time.Hour})
		if err := runner.Start(); err != nil {
			panic(err)
		}
		if err := runner.RunOnMainLoop(func(*actorSimple.SimpleActor) { panic("actor panic marker") }); err != nil {
			panic(err)
		}
		select {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestActorPanicExitsProcess$")
	command.Env = append(os.Environ(), "NOVA_ACTOR_PANIC_TEST=1")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "panic: actor panic marker") {
		t.Fatalf("panic result = %v, output=%s", err, output)
	}
}
