package actor

import (
	"context"
	"errors"
	"sync"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/core/help"
	"golang.org/x/sync/singleflight"
)

type Loader[T actorDef.Actor] func(ctx context.Context, pid actorDef.PID) (T, error)

var ErrSystemStopped = errors.New("actor system stopped")

type System[T actorDef.Actor] struct {
	actorType    actorDef.Type
	loader       Loader[T]
	runnerConfig RunnerConfig

	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.RWMutex
	actors  map[actorDef.Key]*Runner[T]
	loads   singleflight.Group
	loading sync.WaitGroup
	stopped bool
}

func NewSystem[T actorDef.Actor](parent context.Context, actorType actorDef.Type, loader Loader[T], runnerConfig RunnerConfig) *System[T] {
	ctx, cancel := context.WithCancel(parent)
	return &System[T]{
		actorType:    actorType,
		loader:       loader,
		runnerConfig: runnerConfig,
		ctx:          ctx,
		cancel:       cancel,
		actors:       make(map[actorDef.Key]*Runner[T]),
	}
}

func (s *System[T]) TryGetActor(key actorDef.Key) (*Runner[T], bool) {
	s.mu.RLock()
	runner, ex := s.actors[key]
	s.mu.RUnlock()
	if !ex {
		return nil, false
	}
	select {
	case <-runner.Done():
		return nil, false
	default:
	}
	return runner, ex
}

func (s *System[T]) GetOrLoadActor(ctx context.Context, key actorDef.Key) (*Runner[T], error) {
	if s.ctx.Err() != nil {
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
		s.loading.Add(1)
		s.mu.RUnlock()
		defer s.loading.Done()

		if runner, ok := s.TryGetActor(key); ok {
			return runner, nil
		}

		pid := actorDef.PID{Type: s.actorType, Key: key}
		var actorValue T
		var loadErr error

		help.SafeRun(func() {
			actorValue, loadErr = s.loader(s.ctx, pid)
		})
		if loadErr != nil {
			return nil, loadErr
		}
		if s.ctx.Err() != nil {
			return nil, ErrSystemStopped
		}

		runner := NewRunner(s.ctx, pid, actorValue, s.runnerConfig)
		if err := runner.Start(); err != nil {
			return nil, err
		}

		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			_ = runner.Stop(actorDef.StopReasonShutdown)
			return nil, ErrSystemStopped
		}
		s.actors[key] = runner
		s.mu.Unlock()

		go func() {
			<-runner.Done()

			s.mu.Lock()
			if s.actors[key] == runner {
				delete(s.actors, key)
			}
			s.mu.Unlock()
		}()

		return runner, nil
	})

	select {
	case res := <-resultCh:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*Runner[T]), nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *System[T]) UnloadActor(key actorDef.Key) error {
	runner, ok := s.TryGetActor(key)
	if !ok {
		return nil
	}
	return runner.Stop(actorDef.StopReasonUnload)
}

func (s *System[T]) Stop() error {
	s.mu.Lock()
	s.stopped = true
	runners := make([]*Runner[T], 0, len(s.actors))
	for _, runner := range s.actors {
		runners = append(runners, runner)
	}
	s.mu.Unlock()

	s.cancel()
	s.loading.Wait()

	var stopErr error
	for _, runner := range runners {
		stopErr = errors.Join(stopErr, runner.Stop(actorDef.StopReasonShutdown))
	}
	return stopErr
}
