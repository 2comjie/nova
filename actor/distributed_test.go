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

	"github.com/2comjie/nova/actor"
	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	actorSimple "github.com/2comjie/nova/actor/simple"
	"github.com/2comjie/nova/app/node"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	"github.com/2comjie/nova/rpc/rpcerr"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

type messageActor struct {
	actorSimple.SimpleActor
	value byte
}

func (a *messageActor) handle(_ actorDef.PID, ctx *node.Context) error {
	a.value += ctx.Request.Body[0]
	if !ctx.NeedReply() {
		return nil
	}
	return ctx.Reply([]byte{a.value})
}

func (a *messageActor) rpc(_ actorDef.PID, _ context.Context, message actor.Message) ([]byte, error) {
	a.value += message.Body[0]
	return []byte{a.value}, nil
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

	leaseA, _, acquired, err := guardA.TryAcquire(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("player-1 did not acquire actor guard")
	}

	if lease, _, acquired, err := guardB.TryAcquire(context.Background(), pid); err != nil {
		t.Fatal(err)
	} else if acquired {
		_ = lease.Release()
		t.Fatal("player-2 acquired an actor guarded by player-1")
	}

	if err = leaseA.Release(); err != nil {
		t.Fatal(err)
	}
	leaseB, _, acquired, err := guardB.TryAcquire(context.Background(), pid)
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

	oldLease, _, acquired, err := oldGuard.TryAcquire(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("old player-1 did not acquire actor guard")
	}
	newLease, _, acquired, err := newGuard.TryAcquire(context.Background(), pid)
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

	systemA := actor.NewSystem(grpc.NewServer())
	actorsA := actor.Register(systemA, actorDef.Type(1), guardA, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		loadA.Add(1)
		return &actorSimple.SimpleActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	systemB := actor.NewSystem(grpc.NewServer())
	actorsB := actor.Register(systemB, actorDef.Type(1), guardB, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		loadB.Add(1)
		return &actorSimple.SimpleActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	t.Cleanup(func() {
		_ = systemA.Shutdown(context.Background())
		_ = systemB.Shutdown(context.Background())
	})

	if _, err := actorsA.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []actor.ActivationPolicy{actor.ActivationIgnore, actor.ActivationRequire} {
		_, handled, err := actorsB.ResolveActor(context.Background(), actorDef.Key("uid-1001"), policy)
		redirect, ok := err.(rpcerr.Err)
		if handled || !ok || redirect.Code() != actor.ErrorCodeActorRedirect || string(redirect.Detail()) != "player-1" {
			t.Fatalf("policy=%d handled=%t error=%v", policy, handled, err)
		}
	}
	if _, err := actorsB.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); err == nil || err.(rpcerr.Err).Code() != actor.ErrorCodeActorRedirect {
		t.Fatalf("player-2 load error=%v", err)
	}
	if loadA.Load() != 1 || loadB.Load() != 0 {
		t.Fatalf("load count player-1=%d player-2=%d", loadA.Load(), loadB.Load())
	}

	if err := actorsA.UnloadActor(actorDef.Key("uid-1001")); err != nil {
		t.Fatal(err)
	}
	if _, err := actorsB.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); err != nil {
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
	systemA := actor.NewSystem(grpc.NewServer())
	actorsA := actor.Register(systemA, actorDef.Type(1), actorGuard.New("player-1", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix)), loader, actor.RunnerConfig{UpdateDt: time.Hour})
	systemB := actor.NewSystem(grpc.NewServer())
	actorsB := actor.Register(systemB, actorDef.Type(1), actorGuard.New("player-2", rc, actorGuard.WithTTL(600*time.Millisecond), actorGuard.WithKeyPrefix(prefix)), loader, actor.RunnerConfig{UpdateDt: time.Hour})

	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	start := make(chan struct{})
	var workers sync.WaitGroup

	run := func(actors *actor.Manager[*actorSimple.SimpleActor]) {
		defer workers.Done()
		<-start
		for runCtx.Err() == nil {
			runner, err := actors.GetOrLoadActor(runCtx, key)
			if redirect, ok := err.(rpcerr.Err); ok && redirect.Code() == actor.ErrorCodeActorRedirect {
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
			if err = actors.UnloadActor(key); err != nil {
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
	go run(actorsA)
	go run(actorsB)
	close(start)
	workers.Wait()
	if err := systemA.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := systemB.Shutdown(context.Background()); err != nil {
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
	system := actor.NewSystem(grpc.NewServer())
	actors := actor.Register(system, pid.Type, guard, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{
			MStop: func(ctx actorDef.ActorStopCtx) error {
				stopReason <- ctx.Reason
				return nil
			},
		}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})

	runner, err := actors.GetOrLoadActor(context.Background(), pid.Key)
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
	_ = system.Shutdown(context.Background())
	owner, err := rc.Get(context.Background(), guardKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	if owner != "player-2" {
		t.Fatalf("guard owner=%s", owner)
	}
}

func TestSystemShutdownStopsActorWithoutRPCRoutes(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	stopped := make(chan actorDef.StopReason, 1)
	system := actor.NewSystem(grpc.NewServer())
	actors := actor.Register(system, actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*actorSimple.SimpleActor, error) {
		return &actorSimple.SimpleActor{
			MStop: func(ctx actorDef.ActorStopCtx) error {
				stopped <- ctx.Reason
				return nil
			},
		}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})

	if _, err := actors.GetOrLoadActor(context.Background(), actorDef.Key("uid-1001")); err != nil {
		t.Fatal(err)
	}
	if err := system.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reason := <-stopped; reason != actorDef.StopReasonShutdown {
		t.Fatalf("stop reason=%d", reason)
	}
}

func TestActorRPCServerAskTellAndActivation(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	var loads atomic.Int32
	system := actor.NewSystem(grpc.NewServer())
	actors := actor.Register(system, actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		loads.Add(1)
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	actors.RPC().Handle(1001, (*messageActor).rpc)
	t.Cleanup(func() {
		_ = system.Shutdown(context.Background())
	})

	request := &pbActor.Request{ActorType: 1, ActorKey: "uid-1001", Activation: uint32(actor.ActivationIgnore), Route: 1001, Body: []byte{1}}
	response, err := system.Ask(context.Background(), request)
	if err != nil || response.Handled {
		t.Fatalf("ignore handled=%t error=%v", response.Handled, err)
	}
	request.Activation = uint32(actor.ActivationRequire)
	response, err = system.Ask(context.Background(), request)
	rpcError, ok := err.(rpcerr.Err)
	if !ok || rpcError.Code() != actor.ErrorCodeActorNotActive {
		t.Fatalf("require error=%v", err)
	}

	request.Activation = uint32(actor.ActivationLoad)
	response, err = system.Ask(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !response.Handled || len(response.Body) != 1 || response.Body[0] != 1 {
		t.Fatalf("handled=%t body=%v", response.Handled, response.Body)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads=%d", loads.Load())
	}

	request.Activation = uint32(actor.ActivationRequire)
	request.Body = []byte{2}
	if _, err = system.Tell(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Body = []byte{0}
	response, err = system.Ask(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !response.Handled || len(response.Body) != 1 || response.Body[0] != 3 {
		t.Fatalf("handled=%t body=%v", response.Handled, response.Body)
	}
}

func TestActorRPCServerHandlerPanic(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	system := actor.NewSystem(grpc.NewServer())
	actors := actor.Register(system, actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	actors.RPC().Handle(1001, func(*messageActor, actorDef.PID, context.Context, actor.Message) ([]byte, error) {
		panic("handler panic")
	})
	t.Cleanup(func() {
		_ = system.Shutdown(context.Background())
	})

	_, err := system.Ask(context.Background(), &pbActor.Request{
		ActorType: 1, ActorKey: "uid-1001", Activation: uint32(actor.ActivationLoad), Route: 1001,
	})
	rpcError, ok := err.(rpcerr.Err)
	if !ok || rpcError.Code() != actor.ErrorCodeExecutionFailed {
		t.Fatalf("handler error=%v", err)
	}
}

func TestActorRPCBusinessError(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	system := actor.NewSystem(grpc.NewServer())
	actors := actor.Register(system, actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	actors.RPC().Handle(1001, func(*messageActor, actorDef.PID, context.Context, actor.Message) ([]byte, error) {
		return nil, rpcerr.New(10001, "coin not enough")
	})
	t.Cleanup(func() {
		_ = system.Shutdown(context.Background())
	})

	_, err := system.Ask(context.Background(), &pbActor.Request{
		ActorType: 1, ActorKey: "uid-1001", Activation: uint32(actor.ActivationLoad), Route: 1001,
	})
	rpcError, ok := err.(rpcerr.Err)
	if !ok || rpcError.Code() != 10001 || rpcError.Message() != "coin not enough" {
		t.Fatalf("error=%v", err)
	}
}

func TestActorRouteGroup(t *testing.T) {
	rc := localRedis(t)
	guard := actorGuard.New("player-1", rc, actorGuard.WithTTL(time.Second), actorGuard.WithKeyPrefix(actorGuardPrefix()))
	system := actor.NewSystem(grpc.NewServer())
	actors := actor.Register(system, actorDef.Type(1), guard, func(context.Context, actorDef.PID) (*messageActor, error) {
		return &messageActor{}, nil
	}, actor.RunnerConfig{UpdateDt: time.Hour})
	t.Cleanup(func() {
		_ = system.Shutdown(context.Background())
	})

	router := node.NewRouter()
	received := make(chan *node.Context, 4)
	receivedPID := make(chan actorDef.PID, 4)
	middlewareOrder := make(chan string, 16)
	routes := actor.NewRouteGroup(router, actors, actor.ActivationLoad)
	routes.Use(func(next actor.Handler[*messageActor]) actor.Handler[*messageActor] {
		return func(actorValue *messageActor, pid actorDef.PID, ctx *node.Context) error {
			middlewareOrder <- "parent-before"
			err := next(actorValue, pid, ctx)
			middlewareOrder <- "parent-after"
			return err
		}
	})
	child := routes.Group()
	child.Use(func(next actor.Handler[*messageActor]) actor.Handler[*messageActor] {
		return func(actorValue *messageActor, pid actorDef.PID, ctx *node.Context) error {
			middlewareOrder <- "child-before"
			err := next(actorValue, pid, ctx)
			middlewareOrder <- "child-after"
			return err
		}
	})
	child.Handle(1001, func(actorValue *messageActor, pid actorDef.PID, ctx *node.Context) error {
		received <- ctx
		receivedPID <- pid
		return actorValue.handle(pid, ctx)
	})

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
			ActorKey:        "uid-1001",
			NeedReply:       true,
		},
	}
	if err := router.Dispatch(requestCtx); err != nil {
		t.Fatal(err)
	}
	if handledCtx := <-received; handledCtx != requestCtx {
		t.Fatal("actor handler did not receive the original node context")
	}
	if pid := <-receivedPID; pid.Type != actorDef.Type(1) || pid.Key != actorDef.Key("uid-1001") {
		t.Fatalf("actor pid=%+v", pid)
	}
	for index, want := range []string{"parent-before", "child-before", "child-after", "parent-after"} {
		if got := <-middlewareOrder; got != want {
			t.Fatalf("middleware[%d]=%s want=%s", index, got, want)
		}
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
		Request: &node.Request{Route: 1001, UID: "uid-1001", ActorKey: "uid-1001", Body: []byte{2}},
	}
	if err := router.Dispatch(tellCtx); err != nil {
		t.Fatal(err)
	}
	checkCtx := &node.Context{
		Context: context.Background(),
		App:     nodeApp,
		Request: &node.Request{Route: 1001, UID: "uid-1001", ActorKey: "uid-1001", Body: []byte{0}, NeedReply: true},
	}
	if err := router.Dispatch(checkCtx); err != nil {
		t.Fatal(err)
	}
	<-received
	<-received
	<-receivedPID
	<-receivedPID
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
