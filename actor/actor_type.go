package actor

import (
	"context"
	"errors"
	"sync"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/actor/actorGuard"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/rpc/rpcerr"
	"golang.org/x/sync/singleflight"
)

type Loader[T actorDef.Actor] func(runCtx context.Context, pid actorDef.PID) (T, error)

type activation[T actorDef.Actor] struct {
	runner *Runner[T]
	lease  *actorGuard.Lease
	done   chan struct{}
	err    error
}

type Manager[T actorDef.Actor] struct {
	system       *System
	actorType    actorDef.Type
	loader       Loader[T]
	guard        *actorGuard.Guard
	runnerConfig RunnerConfig

	mu     sync.RWMutex
	actors map[actorDef.Key]*activation[T]
	loads  singleflight.Group
}

func Register[T actorDef.Actor](system *System, actorType actorDef.Type, guard *actorGuard.Guard, loader Loader[T], runnerConfig RunnerConfig) *Manager[T] {
	system.registerActorType(actorType)
	return &Manager[T]{system: system, actorType: actorType, loader: loader, guard: guard, runnerConfig: runnerConfig, actors: make(map[actorDef.Key]*activation[T])}
}

func (a *Manager[T]) TryGetActor(key actorDef.Key) (*Runner[T], bool) {
	if a.system.runCtx.Err() != nil {
		return nil, false
	}
	a.mu.RLock()
	activation := a.actors[key]
	a.mu.RUnlock()
	if activation == nil || !activation.runner.Running() {
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

func (a *Manager[T]) ResolveActor(ctx context.Context, key actorDef.Key, policy ActivationPolicy) (*Runner[T], bool, error) {
	switch policy {
	case ActivationLoad:
		runner, err := a.GetOrLoadActor(ctx, key)
		return runner, err == nil, err
	case ActivationIgnore:
		runner, exists := a.TryGetActor(key)
		if exists {
			return runner, true, nil
		}
		ownerInstanceId, err := a.guard.Owner(ctx, actorDef.PID{Type: a.actorType, Key: key})
		if err != nil {
			return nil, false, err
		}
		if ownerInstanceId != "" && ownerInstanceId != a.guard.InstanceId() {
			return nil, false, rpcerr.NewWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+ownerInstanceId, []byte(ownerInstanceId))
		}
		return nil, false, nil
	case ActivationRequire:
		runner, exists := a.TryGetActor(key)
		if exists {
			return runner, true, nil
		}
		ownerInstanceId, err := a.guard.Owner(ctx, actorDef.PID{Type: a.actorType, Key: key})
		if err != nil {
			return nil, false, err
		}
		if ownerInstanceId != "" && ownerInstanceId != a.guard.InstanceId() {
			return nil, false, rpcerr.NewWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+ownerInstanceId, []byte(ownerInstanceId))
		}
		return nil, false, ErrActorNotActive
	default:
		return nil, false, ErrInvalidActivationPolicy
	}
}

func (a *Manager[T]) GetOrLoadActor(waitCtx context.Context, key actorDef.Key) (*Runner[T], error) {
	if a.system.runCtx.Err() != nil {
		return nil, ErrSystemStopped
	}
	if runner, ok := a.TryGetActor(key); ok {
		return runner, nil
	}

	resultCh := a.loads.DoChan(string(key), func() (any, error) {
		if !a.system.beginTask() {
			return nil, ErrSystemStopped
		}
		defer a.system.endTask()

		if runner, ok := a.TryGetActor(key); ok {
			return runner, nil
		}
		a.mu.RLock()
		previous := a.actors[key]
		a.mu.RUnlock()
		if previous != nil {
			select {
			case <-previous.done:
			case <-a.system.runCtx.Done():
				return nil, ErrSystemStopped
			}
		}
		if a.system.runCtx.Err() != nil {
			return nil, ErrSystemStopped
		}

		pid := actorDef.PID{Type: a.actorType, Key: key}
		lease, ownerInstanceId, acquired, err := a.guard.TryAcquire(a.system.runCtx, pid)
		if err != nil {
			return nil, err
		}
		if !acquired {
			return nil, rpcerr.NewWithDetail(ErrorCodeActorRedirect, "actor guarded by instance "+ownerInstanceId, []byte(ownerInstanceId))
		}

		var actorValue T
		var loadErr error
		help.SafeRun(func() {
			actorValue, loadErr = a.loader(a.system.runCtx, pid)
		})
		if loadErr != nil {
			return nil, errors.Join(loadErr, lease.Release())
		}
		if a.system.runCtx.Err() != nil {
			return nil, errors.Join(ErrSystemStopped, lease.Release())
		}
		if lease.Err() != nil {
			return nil, lease.Err()
		}

		runner := NewRunner(a.system.runCtx, pid, actorValue, a.runnerConfig)
		if err = runner.Start(); err != nil {
			return nil, errors.Join(err, lease.Release())
		}
		if lease.Err() != nil {
			_ = runner.Stop(actorDef.StopReasonLeaseLost)
			return nil, errors.Join(lease.Err(), runner.Err())
		}
		if !a.system.beginTask() {
			_ = runner.Stop(actorDef.StopReasonShutdown)
			return nil, errors.Join(ErrSystemStopped, lease.Release())
		}

		current := &activation[T]{runner: runner, lease: lease, done: make(chan struct{})}
		a.mu.Lock()
		a.actors[key] = current
		a.mu.Unlock()

		help.SafeGo(func() {
			defer a.system.endTask()
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
			if a.system.isStopping() {
				a.system.addErr(current.err)
			}

			a.mu.Lock()
			if a.actors[key] == current {
				delete(a.actors, key)
			}
			a.mu.Unlock()
			close(current.done)
		})

		return runner, nil
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

func (a *Manager[T]) UnloadActor(key actorDef.Key) error {
	a.mu.RLock()
	activation := a.actors[key]
	a.mu.RUnlock()
	if activation == nil {
		return nil
	}
	activation.runner.RequestStop(actorDef.StopReasonUnload)
	<-activation.done
	return activation.err
}

func (a *Manager[T]) RPC() *RPCRouteGroup[T] {
	return &RPCRouteGroup[T]{actors: a}
}
