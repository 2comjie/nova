package redisLock

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/redis/go-redis/v9"
)

const lockRetryInterval = 100 * time.Millisecond

var ErrLeaseLost = errors.New("redis lock lease lost")

//go:embed lock.lua
var lockScript string

//go:embed renew.lua
var renewScript string

//go:embed unlock.lua
var unlockScript string

var (
	lockScriptSHA   = redis.NewScript(lockScript)
	renewScriptSHA  = redis.NewScript(renewScript)
	unlockScriptSHA = redis.NewScript(unlockScript)
)

type Lease struct {
	rc        redis.UniversalClient
	lockKey   string
	lockValue string
	ttl       time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	done chan struct{}
}

func TryLockDefault(rc redis.UniversalClient, ctx context.Context, lockKey string) (*Lease, bool, error) {
	return TryLock(rc, ctx, lockKey, 10*time.Second, 5*time.Second)
}
func LockDefault(rc redis.UniversalClient, ctx context.Context, lockKey string) (*Lease, bool, error) {
	return Lock(rc, ctx, lockKey, 10*time.Second)
}

func TryLock(rc redis.UniversalClient, ctx context.Context, lockKey string, ttl time.Duration, timeout time.Duration) (*Lease, bool, error) {
	if err := validateLockArgs(rc, lockKey, ttl); err != nil {
		return nil, false, err
	}
	if timeout < 0 {
		return nil, false, errors.New("redis lock timeout cannot be negative")
	}

	deadline := time.Now().Add(timeout)
	for {
		lease, locked, err := tryAcquire(rc, ctx, lockKey, ttl)
		if err != nil {
			return nil, false, err
		}
		if locked {
			return lease, true, nil
		}
		if timeout == 0 {
			return nil, false, nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, false, nil
		}
		timer := time.NewTimer(min(lockRetryInterval, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false, ctx.Err()
		case <-timer.C:
			if !time.Now().Before(deadline) {
				return nil, false, nil
			}
		}
	}
}

// Lock 持续尝试获取锁，直到成功或者 ctx 被取消。
func Lock(rc redis.UniversalClient, ctx context.Context, lockKey string, ttl time.Duration) (*Lease, bool, error) {
	if err := validateLockArgs(rc, lockKey, ttl); err != nil {
		return nil, false, err
	}

	for {
		lease, locked, err := tryAcquire(rc, ctx, lockKey, ttl)
		if err != nil {
			return nil, false, err
		}
		if locked {
			return lease, true, nil
		}

		timer := time.NewTimer(lockRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false, ctx.Err()
		case <-timer.C:
		}
	}
}

func validateLockArgs(rc redis.UniversalClient, lockKey string, ttl time.Duration) error {
	if rc == nil {
		return errors.New("redis client is nil")
	}
	if lockKey == "" {
		return errors.New("redis lock key is empty")
	}
	if ttl < time.Millisecond {
		return errors.New("redis lock ttl must be at least 1ms")
	}
	return nil
}

func tryAcquire(rc redis.UniversalClient, ctx context.Context, lockKey string, ttl time.Duration) (*Lease, bool, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, false, err
	}
	lockValue := hex.EncodeToString(token[:])

	locked, err := lockScriptSHA.Run(ctx, rc, []string{lockKey}, lockValue, ttl.Milliseconds()).Int64()
	if err != nil {
		return nil, false, err
	}
	if locked == 0 {
		return nil, false, nil
	}

	leaseCtx, cancel := context.WithCancel(context.Background())
	lease := &Lease{
		rc:        rc,
		lockKey:   lockKey,
		lockValue: lockValue,
		ttl:       ttl,
		ctx:       leaseCtx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	help.SafeGo(lease.renew)
	return lease, true, nil
}

// Renew 手动将当前租约延长一个 TTL。
func (l *Lease) Renew(ctx context.Context) error {
	locked, err := renewScriptSHA.Run(
		ctx,
		l.rc,
		[]string{l.lockKey},
		l.lockValue,
		l.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return err
	}
	if locked == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (l *Lease) Unlock(ctx context.Context) error {
	l.cancel()

	select {
	case <-l.done:
	case <-ctx.Done():
		return ctx.Err()
	}

	deleted, err := unlockScriptSHA.Run(
		ctx,
		l.rc,
		[]string{l.lockKey},
		l.lockValue,
	).Int64()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (l *Lease) Done() <-chan struct{} {
	return l.done
}

func (l *Lease) Active() bool {
	select {
	case <-l.done:
		return false
	default:
		return true
	}
}

func (l *Lease) renew() {
	defer close(l.done)

	ticker := time.NewTicker(l.ttl / 3)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			if err := l.Renew(l.ctx); err != nil {
				return
			}
		}
	}
}
