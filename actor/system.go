package actor

import (
	"context"
	"errors"
	"sync"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/actor/actorGuard"
	"github.com/2comjie/wali/core/help"
	"golang.org/x/sync/singleflight"
)

type Loader[T actorDef.Actor] func(runCtx context.Context, pid actorDef.PID) (T, error)

var (
	ErrSystemStopped = errors.New("actor system stopped")
	ErrActorGuarded  = errors.New("actor guarded by another instance")
)

type activation[T actorDef.Actor] struct {
	runner *Runner[T]
	lease  *actorGuard.Lease
	done   chan struct{}
	err    error
}

type System[T actorDef.Actor] struct {
	actorType    actorDef.Type
	loader       Loader[T]
	guard        *actorGuard.Guard
	runnerConfig RunnerConfig

	runCtx context.Context
	stop   context.CancelFunc

	mu      sync.RWMutex
	actors  map[actorDef.Key]*activation[T]
	loads   singleflight.Group
	loadWg  sync.WaitGroup
	stopped bool
}

func NewSystem[T actorDef.Actor](parentCtx context.Context, actorType actorDef.Type, guard *actorGuard.Guard, loader Loader[T], runnerConfig RunnerConfig) *System[T] {
	runCtx, stop := context.WithCancel(parentCtx)
	return &System[T]{
		actorType:    actorType,
		loader:       loader,
		guard:        guard,
		runnerConfig: runnerConfig,
		runCtx:       runCtx,
		stop:         stop,
		actors:       make(map[actorDef.Key]*activation[T]),
	}
}

func (s *System[T]) TryGetActor(key actorDef.Key) (*Runner[T], bool) {
	if s.runCtx.Err() != nil {
		return nil, false
	}
	s.mu.RLock()
	activation := s.actors[key]
	s.mu.RUnlock()
	if activation == nil {
		return nil, false
	}
	select {
	case <-activation.lease.Done():
		return nil, false
	default:
	}
	select {
	case <-activation.runner.Done():
		return nil, false
	default:
	}
	return activation.runner, true
}

func (s *System[T]) GetOrLoadActor(waitCtx context.Context, key actorDef.Key) (*Runner[T], error) {
	if s.runCtx.Err() != nil {
		return nil, ErrSystemStopped
	}
	if runner, ok := s.TryGetActor(key); ok {
		return runner, nil
	}

	resultCh := s.loads.DoChan(string(key), func() (any, error) {
		s.mu.RLock()
		if s.stopped {
			s.mu.RUnlock()
			return nil, ErrSystemStopped
		}
		s.loadWg.Add(1)
		s.mu.RUnlock()
		defer s.loadWg.Done()

		if runner, ok := s.TryGetActor(key); ok {
			return runner, nil
		}
		s.mu.RLock()
		previous := s.actors[key]
		s.mu.RUnlock()
		if previous != nil {
			select {
			case <-previous.done:
			case <-s.runCtx.Done():
				return nil, ErrSystemStopped
			}
		}
		if s.runCtx.Err() != nil {
			return nil, ErrSystemStopped
		}

		pid := actorDef.PID{Type: s.actorType, Key: key}
		lease, acquired, err := s.guard.TryAcquire(s.runCtx, pid)
		if err != nil {
			return nil, err
		}
		if !acquired {
			return nil, ErrActorGuarded
		}

		var actorValue T
		var loadErr error

		help.SafeRun(func() {
			actorValue, loadErr = s.loader(s.runCtx, pid)
		})
		if loadErr != nil {
			return nil, errors.Join(loadErr, lease.Release())
		}
		if s.runCtx.Err() != nil {
			return nil, errors.Join(ErrSystemStopped, lease.Release())
		}
		if lease.Err() != nil {
			return nil, lease.Err()
		}

		runner := NewRunner(s.runCtx, pid, actorValue, s.runnerConfig)
		if err = runner.Start(); err != nil {
			return nil, errors.Join(err, lease.Release())
		}
		if lease.Err() != nil {
			_ = runner.Stop(actorDef.StopReasonLeaseLost)
			return nil, errors.Join(lease.Err(), runner.Err())
		}
		current := &activation[T]{runner: runner, lease: lease, done: make(chan struct{})}

		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			_ = runner.Stop(actorDef.StopReasonShutdown)
			return nil, errors.Join(ErrSystemStopped, lease.Release())
		}
		s.actors[key] = current
		s.mu.Unlock()

		help.SafeGo(func() {
			select {
			case <-lease.Done():
				runner.RequestStop(actorDef.StopReasonLeaseLost)
			case <-runner.Done():
			}
			<-runner.Done()

			leaseErr := lease.Err()
			if leaseErr == nil {
				leaseErr = lease.Release()
			}
			current.err = errors.Join(runner.Err(), leaseErr)

			s.mu.Lock()
			if s.actors[key] == current {
				delete(s.actors, key)
			}
			s.mu.Unlock()
			close(current.done)
		})

		return runner, nil
	})

	select {
	case res := <-resultCh:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*Runner[T]), nil

	case <-waitCtx.Done():
		return nil, waitCtx.Err()
	}
}

func (s *System[T]) UnloadActor(key actorDef.Key) error {
	s.mu.RLock()
	activation := s.actors[key]
	s.mu.RUnlock()
	if activation == nil {
		return nil
	}
	activation.runner.RequestStop(actorDef.StopReasonUnload)
	<-activation.done
	return activation.err
}

func (s *System[T]) Stop() error {
	s.mu.Lock()
	s.stopped = true
	activations := make([]*activation[T], 0, len(s.actors))
	for _, activation := range s.actors {
		activations = append(activations, activation)
	}
	s.mu.Unlock()

	s.stop()
	s.loadWg.Wait()

	var stopErr error
	for _, activation := range activations {
		<-activation.done
		stopErr = errors.Join(stopErr, activation.err)
	}
	return stopErr
}
