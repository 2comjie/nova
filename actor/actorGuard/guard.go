package actorGuard

import (
	"context"
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/core/help"
	"github.com/redis/go-redis/v9"
)

var ErrGuardLost = errors.New("actor guard lost")

//go:embed acquire.lua
var acquireScript string

//go:embed renew.lua
var renewScript string

//go:embed release.lua
var releaseScript string

var (
	acquireScriptSHA = redis.NewScript(acquireScript)
	renewScriptSHA   = redis.NewScript(renewScript)
	releaseScriptSHA = redis.NewScript(releaseScript)
)

type Guard struct {
	instanceId string
	rc         redis.UniversalClient
	ttl        time.Duration
	keyPrefix  string
}

type Option func(*Guard)

func WithTTL(ttl time.Duration) Option {
	return func(guard *Guard) {
		guard.ttl = ttl
	}
}

func WithKeyPrefix(prefix string) Option {
	return func(guard *Guard) {
		guard.keyPrefix = prefix
	}
}

func New(instanceId string, rc redis.UniversalClient, opts ...Option) *Guard {
	guard := &Guard{instanceId: instanceId, rc: rc, ttl: 10 * time.Second, keyPrefix: "actor:guard:"}
	for _, opt := range opts {
		opt(guard)
	}
	return guard
}

func (g *Guard) InstanceId() string {
	return g.instanceId
}

func (g *Guard) TryAcquire(runCtx context.Context, pid actorDef.PID) (*Lease, string, bool, error) {
	key := g.keyPrefix + pid.String()
	ownerInstanceId, err := acquireScriptSHA.Run(runCtx, g.rc, []string{key}, g.instanceId, g.ttl.Milliseconds()).Text()
	if err != nil {
		return nil, "", false, err
	}
	if ownerInstanceId != g.instanceId {
		return nil, ownerInstanceId, false, nil
	}

	renewCtx, stopRenew := context.WithCancel(context.Background())
	lease := &Lease{
		guard:     g,
		key:       key,
		renewCtx:  renewCtx,
		stopRenew: stopRenew,
		done:      make(chan struct{}),
	}
	help.SafeGo(lease.renew)
	return lease, ownerInstanceId, true, nil
}

func (g *Guard) Owner(ctx context.Context, pid actorDef.PID) (string, error) {
	ownerInstanceId, err := g.rc.Get(ctx, g.keyPrefix+pid.String()).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return ownerInstanceId, err
}

type Lease struct {
	guard *Guard
	key   string

	renewCtx  context.Context
	stopRenew context.CancelFunc
	done      chan struct{}

	errMu sync.RWMutex
	err   error
}

func (l *Lease) Done() <-chan struct{} {
	return l.done
}

func (l *Lease) Err() error {
	l.errMu.RLock()
	defer l.errMu.RUnlock()
	return l.err
}

func (l *Lease) Release() error {
	l.stopRenew()
	<-l.done

	released, err := releaseScriptSHA.Run(context.Background(), l.guard.rc, []string{l.key}, l.guard.instanceId).Int64()
	if err != nil {
		return err
	}
	if released == 0 {
		return ErrGuardLost
	}
	return nil
}

func (l *Lease) renew() {
	defer close(l.done)

	ticker := time.NewTicker(l.guard.ttl / 3)
	defer ticker.Stop()

	for {
		select {
		case <-l.renewCtx.Done():
			return
		case <-ticker.C:
			renewed, err := renewScriptSHA.Run(
				l.renewCtx,
				l.guard.rc,
				[]string{l.key},
				l.guard.instanceId,
				l.guard.ttl.Milliseconds(),
			).Int64()
			if err != nil {
				if l.renewCtx.Err() == nil {
					l.setErr(err)
				}
				return
			}
			if renewed == 0 {
				l.setErr(ErrGuardLost)
				return
			}
		}
	}
}

func (l *Lease) setErr(err error) {
	l.errMu.Lock()
	l.err = err
	l.errMu.Unlock()
}
