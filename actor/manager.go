package actor

import (
	"context"
	"sync"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/rpc/rpcerr"
	"golang.org/x/sync/singleflight"
)

type Loader[T actorDef.Actor] func(runCtx context.Context, pid actorDef.Pid) (T, error)

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
		system:       s,
		actorType:    actorType,
		guard:        guard,
		loader:       loader,
		runnerConfig: runnerConfig,
		actors:       make(map[actorDef.Key]*activeActor[T]),
	}

	s.registrations[actorType] = managerRegistration{
		stop:   manager.requestStopAll,
		routes: make(map[uint32]rpcProcessor),
	}
	return manager
}

func (m *Manager[T]) TryGetActor(key actorDef.Key) (*Runner[T], bool) {
	m.mu.RLock()
	active := m.actors[key]
	stopping := m.stopping
	m.mu.RUnlock()
	if stopping || active == nil || !active.runner.Running() {
		return nil, false
	}
	if !active.lease.Active() {
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
		runner, exists := m.TryGetActor(key)
		if exists {
			return runner, true, nil
		}
		ownerInstanceId, err := m.guard.Owner(ctx, actorDef.Pid{Type: m.actorType, Key: key})
		if err != nil {
			return nil, false, err
		}
		if ownerInstanceId != "" && ownerInstanceId != m.guard.InstanceId() {
			return nil, false, rpcerr.NewWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+ownerInstanceId, []byte(ownerInstanceId))
		}
		if policy == ActivationRequire {
			return nil, false, ErrActorNotActive
		}
		return nil, false, nil
	default:
		return nil, false, ErrInvalidActivationPolicy
	}
}

func (m *Manager[T]) GetOrLoadActor(waitCtx context.Context, key actorDef.Key) (*Runner[T], error) {
	if runner, ok := m.TryGetActor(key); ok {
		return runner, nil
	}

	resultCh := m.loads.DoChan(string(key), func() (any, error) {
		return m.activateActor(key)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(*Runner[T]), nil
	case <-waitCtx.Done():
		return nil, waitCtx.Err()
	}
}

func (m *Manager[T]) activateActor(key actorDef.Key) (*Runner[T], error) {
	if !m.system.beginTask() {
		return nil, ErrSystemStopped
	}
	defer m.system.endTask()

	if runner, ok := m.TryGetActor(key); ok {
		return runner, nil
	}
	if previous := m.findActor(key); previous != nil {
		select {
		case <-previous.done:
		case <-m.system.runCtx.Done():
			return nil, ErrSystemStopped
		}
	}

	pid := actorDef.Pid{Type: m.actorType, Key: key}
	lease, err := m.acquireLease(pid)
	if err != nil {
		return nil, err
	}

	runner, err := m.startActor(pid, lease)
	if err != nil {
		_ = lease.Release()
		return nil, err
	}

	active := &activeActor[T]{runner: runner, lease: lease, done: make(chan struct{})}
	if !m.addActor(key, active) {
		runner.Stop(actorDef.StopReasonShutdown)
		_ = lease.Release()
		return nil, ErrSystemStopped
	}

	m.system.tasks.Add(1)
	help.SafeGo(func() {
		m.waitActorExit(key, active)
	})
	return runner, nil
}

func (m *Manager[T]) acquireLease(pid actorDef.Pid) (*actorGuard.Lease, error) {
	lease, ownerInstanceId, acquired, err := m.guard.TryAcquire(m.system.runCtx, pid)
	if err != nil {
		return nil, err
	}
	if acquired {
		return lease, nil
	}
	return nil, rpcerr.NewWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+ownerInstanceId, []byte(ownerInstanceId))
}

func (m *Manager[T]) startActor(pid actorDef.Pid, lease *actorGuard.Lease) (*Runner[T], error) {
	actorValue, err := m.loader(m.system.runCtx, pid)
	if err != nil {
		return nil, err
	}
	if !lease.Active() {
		return nil, actorGuard.ErrGuardLost
	}

	runner := NewRunner(context.Background(), pid, actorValue, m.runnerConfig)
	if err := runner.Start(); err != nil {
		return nil, err
	}
	if lease.Active() {
		return runner, nil
	}

	runner.Stop(actorDef.StopReasonLeaseLost)
	return nil, actorGuard.ErrGuardLost
}

func (m *Manager[T]) waitActorExit(key actorDef.Key, active *activeActor[T]) {
	defer m.system.endTask()
	select {
	case <-active.lease.Done():
		active.runner.RequestStop(actorDef.StopReasonLeaseLost)
	case <-active.runner.Done():
		_ = active.lease.Release()
	}
	<-active.runner.Done()

	m.mu.Lock()
	if m.actors[key] == active {
		delete(m.actors, key)
	}
	m.mu.Unlock()
	close(active.done)
}

func (m *Manager[T]) findActor(key actorDef.Key) *activeActor[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.actors[key]
}

func (m *Manager[T]) addActor(key actorDef.Key, active *activeActor[T]) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return false
	}
	m.actors[key] = active
	return true
}

func (m *Manager[T]) UnloadActor(key actorDef.Key) {
	active := m.findActor(key)
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

func (m *Manager[T]) RPC() *RPCRouteGroup[T] {
	return &RPCRouteGroup[T]{actors: m}
}
