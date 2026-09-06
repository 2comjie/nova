package actor

import (
	"context"
	"errors"
	"sync"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/rpc"
	"golang.org/x/sync/singleflight"
)

type Loader[T actorDef.Actor] func(ctx context.Context, pid actorDef.Pid) (T, error)

type activeActor[T actorDef.Actor] struct {
	runner *Runner[T]
	lease  *actorGuard.Lease
	done   chan struct{}
}

type Manager[T actorDef.Actor] struct {
	system       *System
	actorType    actorDef.Type
	loader       Loader[T]
	guard        *actorGuard.Guard
	runnerConfig RunnerConfig

	mu       sync.RWMutex
	actors   map[actorDef.Key]*activeActor[T]
	stopping bool
	loads    singleflight.Group
}

func (s *System) Register[T actorDef.Actor](actorType actorDef.Type, guard *actorGuard.Guard, loader Loader[T], runnerConfig RunnerConfig) *Manager[T] {
	manager := &Manager[T]{
		system: s, actorType: actorType, guard: guard, loader: loader,
		runnerConfig: runnerConfig, actors: make(map[actorDef.Key]*activeActor[T]),
	}
	s.registrations[actorType] = managerRegistration{
		stop: manager.requestStopAll, routes: make(map[uint32]rpcProcessor),
	}
	return manager
}

func (m *Manager[T]) TryGetActor(key actorDef.Key) (*Runner[T], bool) {
	m.mu.RLock()
	active := m.actors[key]
	stopping := m.stopping
	m.mu.RUnlock()
	if stopping || active == nil || !active.runner.Running() || !active.lease.Active() {
		return nil, false
	}
	return active.runner, true
}

func (m *Manager[T]) ResolveActor(ctx context.Context, key actorDef.Key, policy ActivationPolicy) (*Runner[T], bool, error) {
	switch policy {
	case ActivationLoad:
		runner, err := m.GetOrLoadActor(ctx, key)
		return runner, err == nil, err
	case ActivationIgnore, ActivationRequire:
		if runner, exists := m.TryGetActor(key); exists {
			return runner, true, nil
		}
		owner, err := m.guard.Owner(ctx, actorDef.Pid{Type: m.actorType, Key: key})
		if err != nil {
			return nil, false, err
		}
		if owner != "" && owner != m.guard.InstanceId() {
			return nil, false, rpc.NewErrorWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+owner, []byte(owner))
		}
		if policy == ActivationRequire {
			return nil, false, ErrActorNotActive
		}
		return nil, false, nil
	default:
		return nil, false, ErrInvalidActivationPolicy
	}
}

func (m *Manager[T]) GetOrLoadActor(ctx context.Context, key actorDef.Key) (*Runner[T], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runner, ok := m.TryGetActor(key); ok {
		return runner, nil
	}
	resultCh := m.loads.DoChan(string(key), func() (any, error) { return m.activateActor(key) })
	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(*Runner[T]), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Manager[T]) activateActor(key actorDef.Key) (*Runner[T], error) {
	m.system.lifecycleMu.Lock()
	if m.system.stopping {
		m.system.lifecycleMu.Unlock()
		return nil, ErrSystemStopped
	}
	m.system.tasks.Add(1)
	m.system.lifecycleMu.Unlock()
	defer m.system.tasks.Done()

	if runner, ok := m.TryGetActor(key); ok {
		return runner, nil
	}
	m.mu.RLock()
	previous := m.actors[key]
	m.mu.RUnlock()
	if previous != nil {
		select {
		case <-previous.done:
		case <-m.system.runCtx.Done():
			return nil, ErrSystemStopped
		}
	}
	pid := actorDef.Pid{Type: m.actorType, Key: key}
	lease, owner, acquired, err := m.guard.TryAcquire(m.system.runCtx, pid)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, rpc.NewErrorWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+owner, []byte(owner))
	}

	// The activation owns the lease until the exit watcher takes over.
	releaseLease := true
	defer func() {
		if releaseLease {
			if err := lease.Release(); err != nil && !errors.Is(err, actorGuard.ErrGuardLost) {
				logx.Errorf("actor %s lease release failed: %v", pid, err)
			}
		}
	}()
	loadCtx, cancelLoad := context.WithCancel(m.system.runCtx)
	go func() {
		select {
		case <-lease.Done():
			cancelLoad()
		case <-loadCtx.Done():
		}
	}()
	actorValue, err := m.loader(loadCtx, pid)
	cancelLoad()
	if err != nil {
		return nil, err
	}
	if !lease.Active() {
		return nil, actorGuard.ErrGuardLost
	}

	runner := NewRunner(context.Background(), pid, actorValue, m.runnerConfig)
	active := &activeActor[T]{runner: runner, lease: lease, done: make(chan struct{})}
	m.mu.Lock()
	if m.stopping {
		m.mu.Unlock()
		return nil, ErrSystemStopped
	}
	// Publish before OnStart so shutdown and lease loss can stop initialization.
	m.actors[key] = active
	m.mu.Unlock()

	m.system.tasks.Add(1)
	releaseLease = false
	go func() {
		defer m.system.tasks.Done()
		select {
		case <-lease.Done():
			runner.RequestStop(actorDef.StopReasonLeaseLost)
		case <-runner.Done():
		}
		<-runner.Done()
		// OnStop has completed before ownership becomes available elsewhere.
		if err := lease.Release(); err != nil && !errors.Is(err, actorGuard.ErrGuardLost) {
			logx.Errorf("actor %s lease release failed: %v", pid, err)
		}
		m.mu.Lock()
		delete(m.actors, key)
		m.mu.Unlock()
		close(active.done)
	}()
	if err := runner.Start(); err != nil {
		return nil, err
	}
	return runner, nil
}

// UnloadActor waits for OnStop and lease release. Inside an actor use ctx.Unload.
func (m *Manager[T]) UnloadActor(key actorDef.Key) {
	m.mu.RLock()
	active := m.actors[key]
	m.mu.RUnlock()
	if active == nil {
		return
	}
	active.runner.RequestStop(actorDef.StopReasonUnload)
	<-active.done
}

func (m *Manager[T]) requestStopAll() {
	m.mu.Lock()
	m.stopping = true
	for _, active := range m.actors {
		active.runner.RequestStop(actorDef.StopReasonShutdown)
	}
	m.mu.Unlock()
}

func (m *Manager[T]) RPC() *RPCRouteGroup[T] { return &RPCRouteGroup[T]{actors: m} }
