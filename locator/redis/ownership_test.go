package redisLocator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type evalHook struct {
	script        string
	before, after func(redis.Cmder)
}

func (h evalHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h evalHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h evalHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		if command.Name() != "eval" || command.Args()[1] != h.script {
			return next(ctx, command)
		}
		if h.before != nil {
			h.before(command)
		}
		err := next(ctx, command)
		if h.after != nil {
			h.after(command)
		}
		return err
	}
}

func TestOwnershipStaleUnbindKeepsNewRenewal(t *testing.T) {
	rc := testClient(t)
	renewed := make(chan struct{}, 1)
	rc.AddHook(evalHook{script: renewScript, after: func(command redis.Cmder) {
		if command.Args()[5] == "new" && command.(*redis.Cmd).Val() == int64(1) {
			select {
			case renewed <- struct{}{}:
			default:
			}
		}
	}})
	p := NewProvider(rc, WithPrefix("ownership"), WithTTL(2*time.Second), WithTick(10*time.Millisecond))
	defer p.Close()
	lost := make(chan string, 2)
	p.SetOnBindingLost(func(_, _, value string) { lost <- value })
	ctx := context.Background()
	if _, err := p.Bind(ctx, "gate", "42", "old"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Bind(ctx, "gate", "42", "new"); err != nil {
		t.Fatal(err)
	}
	if err := p.Unbind(ctx, "gate", "42", "old"); err != nil {
		t.Fatal(err)
	}
	p.rw.Lock()
	_, oldExists := p.stopChs[[3]string{"gate", "42", "old"}]
	_, newExists := p.stopChs[[3]string{"gate", "42", "new"}]
	p.rw.Unlock()
	if oldExists || !newExists {
		t.Fatalf("scheduled renewal: old=%v new=%v", oldExists, newExists)
	}
	select {
	case <-renewed:
	case <-time.After(2 * time.Second):
		t.Fatal("new binding was not renewed after stale Unbind")
	}
	if value, err := rc.HGet(ctx, "ownership:hash:gate", "42").Result(); err != nil || value != "new" {
		t.Fatalf("binding=%q err=%v", value, err)
	}
	p.Close()
	close(lost)
	for value := range lost {
		if value == "new" {
			t.Fatal("current ownership incorrectly lost")
		}
	}
}

func TestOwnershipOldProviderCannotRenewReplacement(t *testing.T) {
	server := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer rc.Close()
	a := NewProvider(rc, WithPrefix("ownership"), WithTTL(2*time.Second), WithTick(10*time.Millisecond))
	b := NewProvider(rc, WithPrefix("ownership"), WithTTL(2*time.Second), WithTick(time.Second))
	defer a.Close()
	defer b.Close()
	lost := make(chan string, 1)
	a.SetOnBindingLost(func(_, _, value string) { lost <- value })
	ctx := context.Background()
	if _, err := a.Bind(ctx, "gate", "42", "gate-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Bind(ctx, "gate", "42", "gate-b"); err != nil {
		t.Fatal(err)
	}
	b.Close()
	select {
	case value := <-lost:
		if value != "gate-a" {
			t.Fatalf("lost binding=%s", value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old provider did not report lost ownership")
	}
	server.FastForward(3 * time.Second)
	if err := rc.HGet(ctx, "ownership:hash:gate", "42").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("dead replacement did not expire: %v", err)
	}
}

func TestOwnershipRenewalFailureNotifiesAndStops(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithTTL(2*time.Second), WithTick(10*time.Millisecond))
	defer p.Close()
	lost := make(chan string, 1)
	p.SetOnBindingLost(func(_, _, value string) { lost <- value })
	if _, err := p.Bind(context.Background(), "gate", "42", "gate-a"); err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-lost:
		if value != "gate-a" {
			t.Fatalf("lost binding=%s", value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("renewal failure did not report lost ownership")
	}
	p.rw.Lock()
	_, exists := p.stopChs[[3]string{"gate", "42", "gate-a"}]
	p.rw.Unlock()
	if exists {
		t.Fatal("failed ownership is still scheduled for renewal")
	}
}

func TestOwnershipUnbindFailureStillStopsLocalRenewal(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithTTL(2*time.Second), WithTick(time.Second))
	defer p.Close()
	if _, err := p.Bind(context.Background(), "gate", "42", "gate-a"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Unbind(ctx, "gate", "42", "gate-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Unbind error=%v", err)
	}
	p.rw.Lock()
	_, exists := p.stopChs[[3]string{"gate", "42", "gate-a"}]
	p.rw.Unlock()
	if exists {
		t.Fatal("ended session is still scheduled for renewal after Unbind failed")
	}
}

func TestOwnershipStaleRenewalResultCannotRemoveRebind(t *testing.T) {
	rc := testClient(t)
	oldResult, resume, newRenewed := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	var paused atomic.Bool
	rc.AddHook(evalHook{script: renewScript, after: func(command redis.Cmder) {
		result := command.(*redis.Cmd).Val()
		if result == int64(0) && paused.CompareAndSwap(false, true) {
			close(oldResult)
			<-resume
		} else if result == int64(1) && paused.Load() {
			select {
			case newRenewed <- struct{}{}:
			default:
			}
		}
	}})
	p := NewProvider(rc, WithPrefix("ownership"), WithTTL(2*time.Second), WithTick(20*time.Millisecond))
	defer p.Close()
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	lost := make(chan string, 1)
	p.SetOnBindingLost(func(_, _, value string) { lost <- value })
	ctx := context.Background()
	if _, err := p.Bind(ctx, "gate", "42", "same-session"); err != nil {
		t.Fatal(err)
	}
	p.rw.Lock()
	oldStop := p.stopChs[[3]string{"gate", "42", "same-session"}]
	p.rw.Unlock()
	if err := rc.HSet(ctx, "ownership:hash:gate", "42", "other-node").Err(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldResult:
	case <-time.After(2 * time.Second):
		t.Fatal("old renewal did not finish")
	}
	if _, err := p.Bind(ctx, "gate", "42", "same-session"); err != nil {
		t.Fatal(err)
	}
	p.rw.Lock()
	newStop := p.stopChs[[3]string{"gate", "42", "same-session"}]
	p.rw.Unlock()
	if newStop == nil || newStop == oldStop {
		t.Fatal("rebind did not replace the previous renewal worker")
	}
	select {
	case <-oldStop:
	default:
		t.Fatal("rebind did not stop the previous renewal worker")
	}
	release()
	select {
	case <-newRenewed:
	case <-time.After(2 * time.Second):
		t.Fatal("stale renewal removed the newer binding")
	}
	p.Close()
	select {
	case value := <-lost:
		t.Fatalf("stale result reported lost current ownership: %s", value)
	default:
	}
}

func TestOwnershipRenewalMatchesNameAndKey(t *testing.T) {
	rc := testClient(t)
	blocked, resume := make(chan struct{}), make(chan struct{})
	renewed := make(chan [2]string, 16)
	var paused atomic.Bool
	rc.AddHook(evalHook{script: renewScript, after: func(command redis.Cmder) {
		args := command.Args()
		if args[3] == "ownership:hash:gate" && args[4] == "42" && command.(*redis.Cmd).Val() == int64(0) && paused.CompareAndSwap(false, true) {
			close(blocked)
			<-resume
		}
		if command.(*redis.Cmd).Val() == int64(1) && paused.Load() {
			select {
			case renewed <- [2]string{args[3].(string), args[4].(string)}:
			default:
			}
		}
	}})
	p := NewProvider(rc, WithPrefix("ownership"), WithTTL(2*time.Second), WithTick(10*time.Millisecond))
	defer p.Close()
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	lost := make(chan string, 4)
	p.SetOnBindingLost(func(name, key, value string) { lost <- name + "/" + key + "/" + value })
	ctx := context.Background()
	for _, binding := range [][3]string{
		{"gate", "42", "gate-42"}, {"gate", "99", "gate-99"},
		{"actor", "42", "actor-42"}, {"actor", "99", "actor-99"},
	} {
		if _, err := p.Bind(ctx, binding[0], binding[1], binding[2]); err != nil {
			t.Fatal(err)
		}
	}
	if err := rc.HSet(ctx, "ownership:hash:gate", "42", "replacement").Err(); err != nil {
		t.Fatal(err)
	}
	if err := rc.HSet(ctx, "ownership:hash:actor", "99", "replacement").Err(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("gate/42 renewal did not start")
	}
	// The other name/key workers must progress while gate/42 is blocked.
	select {
	case binding := <-lost:
		if binding != "actor/99/actor-99" {
			t.Fatalf("unexpected lost binding: %s", binding)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked renewal prevented another binding from reporting ownership loss")
	}
	remaining := map[[2]string]bool{
		{"ownership:hash:gate", "99"}: true, {"ownership:hash:actor", "42"}: true,
	}
	deadline := time.After(2 * time.Second)
	for len(remaining) != 0 {
		select {
		case binding := <-renewed:
			delete(remaining, binding)
		case <-deadline:
			t.Fatal("blocked renewal prevented other bindings from renewing")
		}
	}
	release()
	select {
	case binding := <-lost:
		if binding != "gate/42/gate-42" {
			t.Fatalf("unexpected lost binding: %s", binding)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gate/42 ownership loss was not reported")
	}
	p.rw.Lock()
	retained := len(p.stopChs) == 2 && p.stopChs[[3]string{"gate", "99", "gate-99"}] != nil &&
		p.stopChs[[3]string{"actor", "42", "actor-42"}] != nil
	p.rw.Unlock()
	if !retained {
		t.Fatal("renewal did not retain exactly the matching bindings")
	}
	p.Close()
	select {
	case binding := <-lost:
		t.Fatalf("unexpected additional lost binding: %s", binding)
	default:
	}
}

func TestOwnershipCloseWaitsForInFlightRenewal(t *testing.T) {
	rc := testClient(t)
	started, resume, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var pause sync.Once
	rc.AddHook(evalHook{script: renewScript, after: func(redis.Cmder) {
		pause.Do(func() { close(started); <-resume })
	}})
	p := NewProvider(rc, WithTTL(2*time.Second), WithTick(10*time.Millisecond))
	defer p.Close()
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	lost := make(chan string, 1)
	p.SetOnBindingLost(func(_, _, value string) { lost <- value })
	if _, err := p.Bind(context.Background(), "gate", "42", "gate-a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("renewal did not start")
	}
	go func() { p.Close(); close(closed) }()
	select {
	case <-p.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel the provider")
	}
	select {
	case <-closed:
		t.Fatal("Close returned before the in-flight renewal finished")
	default:
	}
	release()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish after releasing the renewal")
	}
	select {
	case value := <-lost:
		t.Fatalf("shutdown reported lost ownership: %s", value)
	default:
	}
}

func TestOwnershipRedisOperationsDoNotBlockOtherBindings(t *testing.T) {
	for _, operation := range []string{"bind", "unbind"} {
		t.Run(operation, func(t *testing.T) {
			rc := testClient(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			started, resume := make(chan struct{}), make(chan struct{})
			var blockNext atomic.Bool
			script := bindScript
			if operation == "unbind" {
				script = unbindScript
			}
			rc.AddHook(evalHook{script: script, before: func(redis.Cmder) {
				if blockNext.CompareAndSwap(true, false) {
					close(started)
					<-resume
				}
			}})
			p := NewProvider(rc, WithTTL(time.Minute), WithTick(30*time.Second))
			defer p.Close()
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			var stopCh chan struct{}
			if operation == "unbind" {
				if _, err := p.Bind(ctx, "gate", "42", "node-a"); err != nil {
					t.Fatal(err)
				}
				p.rw.Lock()
				stopCh = p.stopChs[[3]string{"gate", "42", "node-a"}]
				p.rw.Unlock()
			}
			blockNext.Store(true)
			done := make(chan error, 1)
			go func() {
				if operation == "bind" {
					_, err := p.Bind(ctx, "gate", "42", "node-a")
					done <- err
				} else {
					done <- p.Unbind(ctx, "gate", "42", "node-a")
				}
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("Redis operation did not start")
			}
			if operation == "unbind" {
				select {
				case <-stopCh:
				default:
					t.Fatal("Unbind waited for Redis before stopping local renewal")
				}
			}
			other := make(chan error, 1)
			go func() {
				_, err := p.Bind(ctx, "actor", "99", "node-b")
				other <- err
			}()
			select {
			case err := <-other:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("blocked Redis operation held the renewal map lock")
			}
			release()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("Redis operation did not finish after release")
			}
		})
	}
}

func TestOwnershipCloseRejectsInFlightBind(t *testing.T) {
	rc := testClient(t)
	started, resume := make(chan struct{}), make(chan struct{})
	rc.AddHook(evalHook{script: bindScript, before: func(redis.Cmder) {
		close(started)
		<-resume
	}})
	p := NewProvider(rc, WithTTL(time.Minute), WithTick(30*time.Second))
	defer p.Close()
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	done := make(chan error, 1)
	go func() {
		_, err := p.Bind(context.Background(), "gate", "42", "gate-a")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Bind did not start")
	}
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close waited on an unfinished Bind Redis operation")
	}
	release()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Bind after Close error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bind did not finish after release")
	}
	p.rw.Lock()
	count := len(p.stopChs)
	p.rw.Unlock()
	if count != 0 {
		t.Fatal("Bind started a renewal worker after Close")
	}
}
