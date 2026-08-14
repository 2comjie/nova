package actor

import (
	"context"
	"sync"

	"github.com/2comjie/wali/actor/actorDef"
)

type Loader[T actorDef.Actor] func(ctx context.Context, pid actorDef.PID) (T, error)

type System[T actorDef.Actor] struct {
	actorType    actorDef.Type
	loader       Loader[T]
	runnerConfig RunnerConfig

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.RWMutex
	actors map[actorDef.Key]*Runner[T]
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
	return runner, ex
}
