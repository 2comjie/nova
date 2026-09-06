package actor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/2comjie/nova/actor/actorDef"
)

var (
	ErrRunnerStarted    = errors.New("actor runner already started")
	ErrRunnerStopped    = errors.New("actor runner stopped")
	ErrRunnerNotStarted = errors.New("actor runner not started")
	ErrQueueFull        = errors.New("actor queue full")
)

type RunnerConfig struct {
	QueueCap int
	UpdateDt time.Duration
}

type Runner[T actorDef.Actor] struct {
	self   actorDef.Pid
	actor  T
	config RunnerConfig

	runCtx        context.Context
	cancel        context.CancelFunc
	queue         chan func(T)
	stopRequested chan struct{}
	done          chan struct{}

	stateMu    sync.Mutex
	started    bool
	ready      bool
	stopping   bool
	stopReason actorDef.StopReason
}

func NewRunner[T actorDef.Actor](parentCtx context.Context, self actorDef.Pid, actorValue T, config RunnerConfig) *Runner[T] {
	if config.QueueCap == 0 {
		config.QueueCap = 1024
	}
	if config.UpdateDt == 0 {
		config.UpdateDt = 100 * time.Millisecond
	}
	runCtx, cancel := context.WithCancel(parentCtx)
	return &Runner[T]{
		self: self, actor: actorValue, config: config, runCtx: runCtx, cancel: cancel,
		queue:         make(chan func(T), config.QueueCap),
		stopRequested: make(chan struct{}), done: make(chan struct{}),
		stopReason: actorDef.StopReasonShutdown,
	}
}

func (r *Runner[T]) Start() error {
	r.stateMu.Lock()
	if r.stopping {
		r.stateMu.Unlock()
		return ErrRunnerStopped
	}
	if r.started {
		r.stateMu.Unlock()
		return ErrRunnerStarted
	}
	r.started = true
	r.stateMu.Unlock()
	started := make(chan error, 1)
	go r.loop(started)
	return <-started
}

func (r *Runner[T]) RunOnMainLoop(fn func(T)) error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if r.stopping {
		return ErrRunnerStopped
	}
	if !r.ready {
		return ErrRunnerNotStarted
	}
	select {
	case r.queue <- fn:
		return nil
	default:
		return ErrQueueFull
	}
}

// WaitResultOnMainLoop must not be called from this actor's own loop.
// Cancellation skips pending work; an executing handler must cooperate with ctx.
func (r *Runner[T]) WaitResultOnMainLoop(ctx context.Context, fn func(context.Context, T) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result := make(chan error, 1)
	if err := r.RunOnMainLoop(func(actorValue T) {
		if err := ctx.Err(); err != nil {
			result <- err
			return
		}
		execCtx, cancel := context.WithCancel(ctx)
		stopCancel := context.AfterFunc(r.runCtx, cancel)
		defer stopCancel()
		defer cancel()
		result <- fn(execCtx, actorValue)
	}); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		select {
		case err := <-result:
			return err
		default:
			return ErrRunnerStopped
		}
	}
}

func (r *Runner[T]) Stop(reason actorDef.StopReason) {
	r.RequestStop(reason)
	<-r.done
}

// Shutdown and unload reject new tasks, drain the queue and then call OnStop.
// Lease loss cancels execution immediately and discards the remaining queue.
// An OnStart still in progress is canceled for every stop reason.
func (r *Runner[T]) RequestStop(reason actorDef.StopReason) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if r.stopping {
		if reason == actorDef.StopReasonLeaseLost {
			r.stopReason = reason
			r.cancel()
		}
		return
	}
	r.stopping = true
	r.stopReason = reason
	close(r.stopRequested)
	if !r.ready || reason == actorDef.StopReasonLeaseLost {
		r.cancel()
	}
	if !r.started {
		close(r.done)
	}
}

func (r *Runner[T]) loop(started chan<- error) {
	defer func() {
		r.stateMu.Lock()
		if !r.stopping {
			r.stopping = true
			close(r.stopRequested)
		}
		r.ready = false
		r.cancel()
		r.stateMu.Unlock()
		close(r.done)
	}()
	if err := r.actor.OnStart(actorDef.ActorStartCtx{
		Context: r.runCtx, Self: r.self,
		Unload: func() { r.RequestStop(actorDef.StopReasonUnload) },
	}); err != nil {
		started <- err
		return
	}
	defer func() {
		r.stateMu.Lock()
		reason := r.stopReason
		r.stateMu.Unlock()
		r.actor.OnStop(actorDef.ActorStopCtx{Context: r.runCtx, Self: r.self, Reason: reason})
	}()
	if r.runCtx.Err() != nil {
		r.RequestStop(actorDef.StopReasonShutdown)
	}
	r.stateMu.Lock()
	stopping := r.stopping
	r.ready = !stopping
	r.stateMu.Unlock()
	if stopping {
		started <- ErrRunnerStopped
	} else {
		started <- nil
	}

	lastUpdate := time.Now()
	lastActive := lastUpdate
	timer := time.NewTimer(r.config.UpdateDt)
	defer timer.Stop()
	updatePaused := false
	for {
		r.stateMu.Lock()
		stopping := r.stopping
		lost := r.stopReason == actorDef.StopReasonLeaseLost
		r.stateMu.Unlock()
		if lost {
			return
		}
		var fn func(T)
		if stopping {
			select {
			case fn = <-r.queue:
			default:
				return
			}
		} else {
			select {
			case <-r.stopRequested:
				continue
			case <-r.runCtx.Done():
				r.RequestStop(actorDef.StopReasonShutdown)
				continue
			case fn = <-r.queue:
			case now := <-timer.C:
				if !r.Running() {
					continue
				}
				next := r.actor.OnUpdate(actorDef.ActorUpdateCtx{
					Context: r.runCtx, Self: r.self, Delta: now.Sub(lastUpdate), Idle: now.Sub(lastActive),
					Unload: func() { r.RequestStop(actorDef.StopReasonUnload) },
				})
				lastUpdate = now
				if next < 0 {
					updatePaused = true
					continue
				}
				if next == 0 {
					next = r.config.UpdateDt
				}
				timer.Reset(next)
				continue
			}
		}
		// Dequeue does not start execution: lease loss still takes precedence.
		r.stateMu.Lock()
		lost = r.stopReason == actorDef.StopReasonLeaseLost
		r.stateMu.Unlock()
		if lost {
			return
		}
		lastActive = time.Now()
		fn(r.actor)
		if updatePaused {
			updatePaused = false
			timer.Reset(r.config.UpdateDt)
		}
	}
}

func (r *Runner[T]) Done() <-chan struct{} { return r.done }

func (r *Runner[T]) Running() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.ready && !r.stopping
}
