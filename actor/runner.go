package actor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/core/help"
)

type RunnerConfig struct {
	QueueCap int
	UpdateDt time.Duration
}

type Runner[T actorDef.Actor] struct {
	self   actorDef.PID
	actor  T
	config RunnerConfig

	ctx    context.Context
	cancel context.CancelFunc
	queue  chan func(T)
	done   chan struct{}
	start  chan error

	stateMu sync.Mutex
	started bool
	stopped bool

	stopReason actorDef.StopReason

	errMu sync.Mutex
	err   error
}

func NewRunner[T actorDef.Actor](parent context.Context, self actorDef.PID, actorValue T, config RunnerConfig) *Runner[T] {
	if config.QueueCap == 0 {
		config.QueueCap = 1024
	}
	if config.UpdateDt == 0 {
		config.UpdateDt = 100 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(parent)
	return &Runner[T]{
		self:       self,
		actor:      actorValue,
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		queue:      make(chan func(T), config.QueueCap),
		done:       make(chan struct{}),
		start:      make(chan error, 1),
		stopReason: actorDef.StopReasonShutdown,
	}
}

func (r *Runner[T]) Start() error {
	r.stateMu.Lock()
	if r.started {
		r.stateMu.Unlock()
		return errors.New("already started")
	}
	if r.stopped {
		r.stateMu.Unlock()
		return errors.New("already stopped")
	}
	r.started = true
	r.stateMu.Unlock()
	help.SafeGo(r.loop)
	return <-r.start
}

func (r *Runner[T]) RunOnMainLoop(fn func(T)) error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if !r.started {
		return errors.New("not started")
	}
	if r.stopped {
		return errors.New("already stopped")
	}

	select {
	case r.queue <- fn:
		return nil
	default:
		return errors.New("queue full")
	}
}

func (r *Runner[T]) WaitResultOnMainLoop(ctx context.Context, fn func(T)) error {
	done := make(chan struct{}, 1)

	err := r.RunOnMainLoop(func(actorValue T) {
		defer func() {
			done <- struct{}{}
		}()

		if ctx.Err() != nil {
			return
		}

		fn(actorValue)
	})
	if err != nil {
		return err
	}

	select {
	case <-done:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		if err := r.Err(); err != nil {
			return err
		}
		return errors.New("runner stopped")
	}
}

func (r *Runner[T]) Stop(reason actorDef.StopReason) error {
	r.stateMu.Lock()

	if !r.stopped {
		r.stopped = true
		r.stopReason = reason
		r.cancel()
	}

	started := r.started
	r.stateMu.Unlock()

	if !started {
		return nil
	}

	<-r.done
	return r.Err()
}

func (r *Runner[T]) loop() {
	defer close(r.done)

	startErr := errors.New("actor start panic")
	help.SafeRun(func() {
		startErr = r.actor.OnStart(actorDef.ActorStartCtx{
			Context: r.ctx,
			Self:    r.self,
		})
	})

	if startErr != nil {
		r.setErr(startErr)
		r.markStopped()
		r.start <- startErr
		return
	}

	r.start <- nil

	defer func() {
		r.markStopped()

		stopErr := errors.New("actor stop panic")
		help.SafeRun(func() {
			stopErr = r.actor.OnStop(actorDef.ActorStopCtx{
				Context: context.WithoutCancel(r.ctx),
				Self:    r.self,
				Reason:  r.stopReason,
			})
		})

		if stopErr != nil {
			r.setErr(stopErr)
		}
	}()

	lastUpdate := time.Now()
	lastActive := lastUpdate
	timer := time.NewTimer(r.config.UpdateDt)
	defer timer.Stop()

	updatePaused := false
	for {
		select {
		case <-r.ctx.Done():
			return
		case fn := <-r.queue:
			lastActive = time.Now()
			help.SafeRun(func() {
				fn(r.actor)
			})

			if updatePaused {
				updatePaused = false
				timer.Reset(r.config.UpdateDt)
			}

		case now := <-timer.C:
			nextUpdate := r.config.UpdateDt
			help.SafeRun(func() {
				nextUpdate = r.actor.OnUpdate(actorDef.ActorUpdateCtx{
					Context: r.ctx,
					Self:    r.self,
					Delta:   now.Sub(lastUpdate),
					Idle:    now.Sub(lastActive),
				})
				return
			})

			lastUpdate = now
			if nextUpdate < 0 {
				updatePaused = true
				continue
			}
			if nextUpdate == 0 {
				nextUpdate = r.config.UpdateDt
			}
			timer.Reset(nextUpdate)
		}
	}
}

func (r *Runner[T]) setErr(err error) {
	r.errMu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.errMu.Unlock()
}

func (r *Runner[T]) markStopped() {
	r.stateMu.Lock()
	r.stopped = true
	r.cancel()
	r.stateMu.Unlock()
}

func (r *Runner[T]) Done() <-chan struct{} {
	return r.done
}

func (r *Runner[T]) Err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}
