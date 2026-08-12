package actorLock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/locator"
	redisLock "github.com/2comjie/wali/lock/redis"
	"github.com/redis/go-redis/v9"
)

type Ownership struct {
	instanceId     string               // 实例的ID
	locator        *locator.NodeLocator // 定位actor节点信息
	rc             redis.UniversalClient
	leaseTTL       time.Duration
	acquireTimeout time.Duration

	redisLockFmt string
}

type Option func(*Ownership)

func WithLeaseTTL(ttl time.Duration) Option {
	return func(ownership *Ownership) {
		if ttl > 0 {
			ownership.leaseTTL = ttl
		}
	}
}
func WithAcquireTimeout(timeout time.Duration) Option {
	return func(ownership *Ownership) {
		if timeout >= 0 {
			ownership.acquireTimeout = timeout
		}
	}
}
func WithRedisLockFmt(lockFmt string) Option {
	return func(ownership *Ownership) {
		if lockFmt != "" {
			ownership.redisLockFmt = lockFmt
		}
	}
}
func NewOwnership(instanceId string, rc redis.UniversalClient, locator *locator.NodeLocator, opts ...Option) *Ownership {
	if instanceId == "" {
		panic("instanceId can not be empty")
	}
	o := &Ownership{}
	o.instanceId = instanceId
	o.rc = rc
	o.locator = locator
	o.leaseTTL = 10 * time.Second
	o.redisLockFmt = "actor:guard:"
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *Ownership) Locate(ctx context.Context, pid actorDef.PID) (string, error) {
	ins, err := o.locator.Locate(ctx, pid.BindingName(), pid.BindingKey())
	if err != nil {
		return "", err
	}
	return ins, nil
}
func (o *Ownership) TryAcquire(ctx context.Context, pid actorDef.PID) (*OwnerLease, error) {
	lease, acquired, err := redisLock.TryLock(o.rc, ctx, fmt.Sprintf(o.redisLockFmt, pid.String()), o.leaseTTL, o.acquireTimeout)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, nil
	}
	return &OwnerLease{
		pid:        pid,
		instanceId: o.instanceId,
		locator:    o.locator,
		guard:      lease,
	}, nil
}

type OwnerLease struct {
	pid        actorDef.PID
	instanceId string               // 实例的ID
	locator    *locator.NodeLocator // 定位actor节点信息

	guard *redisLock.Lease
	bound atomic.Bool
	once  sync.Once
	err   error
}

func (l *OwnerLease) Bind(ctx context.Context) error {
	if l.bound.Load() {
		return nil
	}
	err := l.locator.Bind(ctx, l.pid.BindingName(), l.pid.BindingKey(), l.instanceId)
	if err != nil {
		return err
	}
	l.bound.Store(true)
	return nil
}
func (l *OwnerLease) Done(ctx context.Context) <-chan struct{} {
	return l.guard.Done()
}
func (l *OwnerLease) Err() error {
	return l.guard.Err()
}
func (l *OwnerLease) Release(ctx context.Context) error {
	l.once.Do(func() {
		var unbindErr error
		if l.bound.CompareAndSwap(true, false) {
			unbindErr = l.locator.Unbind(
				ctx,
				l.pid.BindingName(),
				l.pid.BindingKey(),
				l.instanceId,
			)
		}
		l.err = errors.Join(unbindErr, l.guard.Unlock(ctx))
	})
	return l.err
}
