package actor_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/wali/actor"
	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/actor/actorGuard"
	actorSimple "github.com/2comjie/wali/actor/simple"
	"github.com/2comjie/wali/app/node"
	"github.com/redis/go-redis/v9"
)

type messageActor struct {
	actorSimple.SimpleActor
	value byte
}

func (a *messageActor) ask(ctx *node.Context) error {
	a.value += ctx.Request.Body[0]
	return ctx.Reply([]byte{a.value})
}

func (a *messageActor) tell(ctx *node.Context) error {
	a.value += ctx.Request.Body[0]
	return nil
}

func localRedis(t *testing.T) *redis.Client {
	t.Helper()
	address := os.Getenv("REDIS_ADDR")
	if address == "" {
		address = "127.0.0.1:6379"
	}
	rc := redis.NewClient(&redis.Options{
		Addr:         address,
		DialTimeout:  500 * time.Millisecond,
		ReadTimeout:  500 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,
	})
	if err := rc.Ping(context.Background()).Err(); err != nil {
		_ = rc.Close()
		t.Skipf("redis not available: %v", err)
	}
	t.Cleanup(func() {
		_ = rc.Close()
	})
	return rc
}

func actorGuardPrefix() string {
	return fmt.Sprintf("test:actor:guard:%d:", time.Now().UnixNano())
}

func TestActorGuardAcrossInstances(t *testing.T) {
	rc := localRedis(t)
	prefix := actorGuardPrefix()
	pid := actorDef.PID{Type: actorDef.Type(1), Key: actorDef.Key("uid-1001")}
	guardA := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(prefix))
	guardB := actorGuard.New("player-2", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(prefix))

	leaseA, acquired, err := guardA.TryAcquire(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("player-1 did not acquire actor guard")
	}

	if lease, acquired, err := guardB.TryAcquire(context.Background(), pid); err != nil {
		t.Fatal(err)
	} else if acquired {
		_ = lease.Release()
		t.Fatal("player-2 acquired an actor guarded by player-1")
	}

	if err = leaseA.Release(); err != nil {
		t.Fatal(err)
	}
	leaseB, acquired, err := guardB.TryAcquire(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("player-2 did not acquire the released actor guard")
	}
	if err = leaseB.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestActorGuardReentrantOnSameInstance(t *testing.T) {
	rc := localRedis(t)
	prefix := actorGuardPrefix()
	pid := actorDef.PID{Type: actorDef.Type(1), Key: actorDef.Key("uid-1001")}
	oldGuard := actorGuard.New("player-1", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix))
	newGuard := actorGuard.New("player-1", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix))

	oldLease, acquired, err := oldGuard.TryAcquire(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("old player-1 did not acquire actor guard")
	}
	newLease, acquired, err := newGuard.TryAcquire(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("restarted player-1 could not reenter actor guard")
	}

	if err = newLease.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldLease.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("old lease did not detect guard loss")
	}
	if !errors.Is(oldLease.Err(), actorGuard.ErrGuardLost) {
		t.Fatalf("old lease error=%v", oldLease.Err())
	}
}

func TestSystemDistributedSingleton(t *testing.T) {
	rc := localRedis(t)
	prefix := actorGuardPrefix()
	guardA := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(prefix))
	guardB := actorGuard.New("player-2", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(prefix))
	var loadA atomic.Int32
	var loadB atomic.Int32

	systemA := actor.NewSystem(context.Background(), actorDef.Type(1), guardA, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		loadA.Add(1)
		return &actorSimple.SimpleActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	systemB := actor.NewSystem(context.Background(), actorDef.Type(1), guardB, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		loadB.Add(1)
		return &actorSimple.SimpleActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	t.Cleanup(func() {
		_ = systemA.Stop()
		_ = systemB.Stop()
	})

	if _, err := systemA.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); err != nil {
		t.Fatal(err)
	}
	if _, err := systemB.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); !errors.Is(err, actor.ErrActorGuarded) {
		t.Fatalf("player-2 load error=%v", err)
	}
	if loadA.Load() != 1 || loadB.Load() != 0 {
		t.Fatalf("load count player-1=%d player-2=%d", loadA.Load(), loadB.Load())
	}

	if err := systemA.UnloadActor(actorDef.Key("uid-1001")); err != nil {
		t.Fatal(err)
	}
	if _, err := systemB.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); err != nil {
		t.Fatal(err)
	}
	if loadB.Load() != 1 {
		t.Fatalf("player-2 load count=%d", loadB.Load())
	}
}

func TestSystemDistributedSingletonUnderContention(t *testing.T) {
	rc := localRedis(t)
	prefix := actorGuardPrefix()
	key := actorDef.Key("uid-1001")
	var active atomic.Int32
	var starts atomic.Int32
	var calls atomic.Int32
	var overlapped atomic.Bool

	loader := func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{
			MStart: func(actorDef.ActorStartCtx) error {
				starts.Add(1)
				if active.Add(1) != 1 {
					overlapped.Store(true)
				}
				return nil
			},
			MStop: func(actorDef.ActorStopCtx) error {
				active.Add(-1)
				return nil
			},
		}, nil
	}
	systemA := actor.NewSystem(context.Background(), actorDef.Type(1), actorGuard.New("player-1", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix)), loader, actor.RunnerConfig{UpdateDt: time.Hour})
	systemB := actor.NewSystem(context.Background(), actorDef.Type(1), actorGuard.New("player-2", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix)), loader, actor.RunnerConfig{UpdateDt: time.Hour})

	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	start := make(chan struct{})
	var workers sync.WaitGroup

	run := func(system *actor.System[*actorSimple.SimpleActor]) {
		defer workers.Done()
		<-start
		for runCtx.Err() == nil {
			runner, err := system.GetOrLoadActor(runCtx, key)
			if errors.Is(err, actor.ErrActorGuarded) {
				time.Sleep(time.Millisecond)
				continue
			}
			if err != nil {
				if runCtx.Err() == nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
				}
				return
			}
			if err = runner.WaitResultOnMainLoop(runCtx, func(*actorSimple.SimpleActor) {
				calls.Add(1)
			}); err != nil {
				if runCtx.Err() == nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
				}
				return
			}
			if err = system.UnloadActor(key); err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}

	workers.Add(2)
	go run(systemA)
	go run(systemB)
	close(start)
	workers.Wait()
	if err := systemA.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := systemB.Stop(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
	if overlapped.Load() {
		t.Fatal("the same actor was active on two instances")
	}
	if active.Load() != 0 {
		t.Fatalf("active actor count=%d", active.Load())
	}
	if starts.Load() < 2 || calls.Load() < 2 {
		t.Fatalf("starts=%d calls=%d", starts.Load(), calls.Load())
	}
}

func TestSystemStopsActorAfterGuardLost(t *testing.T) {
	rc := localRedis(t)
	prefix := actorGuardPrefix()
	pid := actorDef.PID{Type: actorDef.Type(1), Key: actorDef.Key("uid-1001")}
	stopReason := make(chan actorDef.StopReason, 1)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix))
	system := actor.NewSystem(context.Background(), pid.Type, guard, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{
			MStop: func(ctx actorDef.ActorStopCtx) error {
				stopReason <- ctx.Reason
				return nil
			},
		}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})

	runner, err := system.GetOrLoadActor(context.Background(), pid.Key)
	if err != nil {
		t.Fatal(err)
	}
	guardKey := prefix + pid.String()
	if err = rc.Set(context.Background(), guardKey, "player-2", time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("actor did not stop after guard loss")
	}
	if reason := <-stopReason; reason != actorDef.StopReasonLeaseLost {
		t.Fatalf("stop reason=%d", reason)
	}
	_ = system.Stop()
	owner, err := rc.Get(context.Background(), guardKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	if owner != "player-2" {
		t.Fatalf("guard owner=%s", owner)
	}
}

func TestActorServerAskTellAndActivation(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	var loads atomic.Int32
	system := actor.NewSystem(context.Background(), actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		loads.Add(1)
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	t.Cleanup(func() {
		_ = system.Stop()
	})

	server := actor.NewServer()
	if err := actor.RegAsk(server, system, 1001, (*messageActor).ask); err != nil {
		t.Fatal(err)
	}
	if err := actor.RegTell(server, system, 1002, (*messageActor).tell); err != nil {
		t.Fatal(err)
	}
	if err := actor.RegTell(server, system, 1001, func(*messageActor, *node.Context) error {
		return nil
	}); !errors.Is(err, actor.ErrMessageRouteRegistered) {
		t.Fatalf("duplicate route error=%v", err)
	}

	key := actorDef.Key("uid-1001")
	message := actor.Message{Route: 1001, Body: []byte{1}}
	if _, handled, err := server.Ask(context.Background(), key, actor.ActivationIgnore, message); err != nil || handled {
		t.Fatalf("ignore handled=%t error=%v", handled, err)
	}
	if _, _, err := server.Ask(context.Background(), key, actor.ActivationRequire, message); !errors.Is(err, actor.ErrActorNotActive) {
		t.Fatalf("require error=%v", err)
	}

	body, handled, err := server.Ask(context.Background(), key, actor.ActivationLoad, message)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(body) != 1 || body[0] != 1 {
		t.Fatalf("handled=%t body=%v", handled, body)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads=%d", loads.Load())
	}

	if err = server.Tell(context.Background(), key, actor.ActivationRequire, actor.Message{Route: 1002, Body: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	body, handled, err = server.Ask(context.Background(), key, actor.ActivationRequire, actor.Message{Route: 1001, Body: []byte{0}})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(body) != 1 || body[0] != 3 {
		t.Fatalf("handled=%t body=%v", handled, body)
	}
}

func TestActorServerHandlerPanic(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	system := actor.NewSystem(context.Background(), actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	t.Cleanup(func() {
		_ = system.Stop()
	})

	server := actor.NewServer()
	if err := actor.RegAsk(server, system, 1001, func(*messageActor, *node.Context) error {
		panic("handler panic")
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err := server.Ask(context.Background(), actorDef.Key("uid-1001"), actor.ActivationLoad, actor.Message{Route: 1001})
	if !errors.Is(err, actor.ErrMessageHandlerPanic) {
		t.Fatalf("handler error=%v", err)
	}
}

func TestActorRouteGroup(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	system := actor.NewSystem(context.Background(), actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	t.Cleanup(func() {
		_ = system.Stop()
	})

	router := node.NewRouter()
	server := actor.NewServer()
	received := make(chan *node.Context, 1)
	routes := actor.NewRouteGroup(router, server, system, actor.ActivationLoad, func(ctx *node.Context) actorDef.Key {
		return actorDef.Key(ctx.Request.UID)
	})
	if err := routes.RegAsk(1001, func(actorValue *messageActor, ctx *node.Context) error {
		received <- ctx
		return actorValue.ask(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	if err := routes.RegTell(1002, (*messageActor).tell); err != nil {
		t.Fatal(err)
	}

	var fallback atomic.Int32
	if err := router.Handle(2001, func(*node.Context) error {
		fallback.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Freeze(); err != nil {
		t.Fatal(err)
	}

	nodeApp := &node.Node{}
	requestCtx := &node.Context{
		Context: context.Background(),
		App:     nodeApp,
		Request: &node.Request{
			Route:           1001,
			UID:             "uid-1001",
			Body:            []byte{1},
			GateServiceName: "gate",
			GateInstanceID:  "gate-1",
			NeedReply:       true,
		},
	}
	if err := router.Dispatch(requestCtx); err != nil {
		t.Fatal(err)
	}
	if handledCtx := <-received; handledCtx != requestCtx {
		t.Fatal("actor handler did not receive the original node context")
	}
	if requestCtx.App != nodeApp || requestCtx.Request.GateServiceName != "gate" || requestCtx.Request.GateInstanceID != "gate-1" {
		t.Fatal("node context metadata was not preserved")
	}
	if body := requestCtx.ResponseBody(); len(body) != 1 || body[0] != 1 {
		t.Fatalf("response body=%v", body)
	}

	tellCtx := &node.Context{
		Context: context.Background(),
		App:     nodeApp,
		Request: &node.Request{Route: 1002, UID: "uid-1001", Body: []byte{2}},
	}
	if err := router.Dispatch(tellCtx); err != nil {
		t.Fatal(err)
	}
	checkCtx := &node.Context{
		Context: context.Background(),
		App:     nodeApp,
		Request: &node.Request{Route: 1001, UID: "uid-1001", Body: []byte{0}, NeedReply: true},
	}
	if err := router.Dispatch(checkCtx); err != nil {
		t.Fatal(err)
	}
	<-received
	if body := checkCtx.ResponseBody(); len(body) != 1 || body[0] != 3 {
		t.Fatalf("response body after tell=%v", body)
	}

	if err := router.Dispatch(&node.Context{Context: context.Background(), Request: &node.Request{Route: 2001}}); err != nil {
		t.Fatal(err)
	}
	if fallback.Load() != 1 {
		t.Fatalf("fallback calls=%d", fallback.Load())
	}
}
