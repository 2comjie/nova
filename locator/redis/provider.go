package redisLocator

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/logx"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
)

//go:embed unbind.lua
var unbindScript string

//go:embed bind.lua
var bindScript string

//go:embed restore.lua
var restoreScript string

type Provider struct {
	rc     redis.UniversalClient
	option *option

	ctx    context.Context
	cancel context.CancelFunc

	stopChs map[string]chan struct{}
	rw      sync.RWMutex

	cache *ristretto.Cache[string, string]
}

func NewProvider(rc redis.UniversalClient, opts ...Option) *Provider {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: o.cacheMaxCost * 10,
		MaxCost:     o.cacheMaxCost,
		BufferItems: 64,
	})
	if err != nil {
		logx.Fatalf("create ristretto cache err %+v", err)
	}

	p := &Provider{
		rc:      rc,
		option:  o,
		ctx:     ctx,
		cancel:  cancel,
		stopChs: make(map[string]chan struct{}),
		cache:   cache,
	}

	help.SafeGo(p.watchNotify)
	return p
}

func (p *Provider) Bind(ctx context.Context, name string, key string, value string) (string, error) {
	p.rw.Lock()
	defer p.rw.Unlock()

	logCtx := logx.WithField("name", name).WithField("key", key)
	previous, err := p.rc.Eval(
		ctx,
		bindScript,
		[]string{p.hashKey(name), p.nameSetKey()},
		key,
		value,
		int(p.option.ttl.Seconds()),
		name,
	).Text()
	if err != nil {
		logCtx.Errorf("eval swap script err %+v", err)
		return "", err
	}

	p.cache.Del(name + ":" + key)
	p.publishEvent("bind", name, key)
	p.startKeepAliveLocked(name, key)
	logCtx.Debugf("swap bind success")
	return previous, nil
}

func (p *Provider) Restore(
	ctx context.Context,
	name string,
	key string,
	current string,
	previous string,
) (bool, error) {
	p.rw.Lock()
	defer p.rw.Unlock()

	logCtx := logx.WithField("name", name).WithField("key", key)
	result, err := p.rc.Eval(
		ctx,
		restoreScript,
		[]string{p.hashKey(name), p.nameSetKey()},
		key,
		current,
		previous,
		int(p.option.ttl.Seconds()),
		name,
	).Int()
	if err != nil {
		logCtx.Errorf("eval restore script err %+v", err)
		return false, err
	}
	if result == 0 {
		return false, nil
	}

	p.cache.Del(name + ":" + key)
	p.publishEvent("bind", name, key)
	p.stopKeepAliveLocked(name, key)
	logCtx.Debugf("restore bind success")
	return true, nil
}

func (p *Provider) Unbind(ctx context.Context, name string, key string, instanceID string) error {
	p.rw.Lock()
	defer p.rw.Unlock()

	logCtx := logx.WithField("name", name).WithField("key", key)
	result, err := p.rc.Eval(
		ctx,
		unbindScript,
		[]string{p.hashKey(name), p.nameSetKey()},
		key,
		instanceID,
		name,
	).Int()
	if err != nil {
		logCtx.Errorf("eval unbind script err %+v", err)
		return err
	}

	p.stopKeepAliveLocked(name, key)
	if result != 0 {
		p.cache.Del(name + ":" + key)
		p.publishEvent("unbind", name, key)
	}
	logCtx.Debugf("unbind success")
	return nil
}

func (p *Provider) Locate(ctx context.Context, name string, key string) (string, error) {
	localKey := name + ":" + key
	if id, ok := p.cache.Get(localKey); ok {
		return id, nil
	}

	id, err := p.rc.HGet(ctx, p.hashKey(name), key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}

	p.cache.SetWithTTL(localKey, id, 1, p.cacheTTL())
	return id, nil
}

func (p *Provider) Close() {
	p.rw.Lock()
	defer p.rw.Unlock()
	p.cancel()
	for key, ch := range p.stopChs {
		close(ch)
		delete(p.stopChs, key)
	}
	p.cache.Close()
}

func (p *Provider) startKeepAliveLocked(name string, key string) {
	stopKey := name + ":" + key
	if p.stopChs[stopKey] != nil {
		return
	}
	stopCh := make(chan struct{})
	p.stopChs[stopKey] = stopCh
	help.SafeGo(func() {
		p.keepAlive(name, key, stopCh)
	})
}

func (p *Provider) stopKeepAliveLocked(name string, key string) {
	stopKey := name + ":" + key
	if ch, ok := p.stopChs[stopKey]; ok {
		close(ch)
		delete(p.stopChs, stopKey)
	}
}

func (p *Provider) keepAlive(name string, key string, stopCh chan struct{}) {
	tk := time.NewTicker(p.option.tick)
	defer tk.Stop()
	logCtx := logx.WithField("name", name).WithField("key", key)
	for {
		select {
		case <-stopCh:
			return
		case <-p.ctx.Done():
			return
		case <-tk.C:
			if err := help.Retry(p.ctx, 3, time.Second, func() error {
				_, err := p.rc.HExpire(p.ctx, p.hashKey(name), p.option.ttl, key).Result()
				return err
			}); err != nil {
				logCtx.Errorf("keep alive failed: %v", err)
			}
		}
	}
}

func (p *Provider) cacheTTL() time.Duration {
	if p.option.ttl <= 0 {
		return 0
	}
	ttl := p.option.ttl / 2
	if ttl <= 0 {
		return p.option.ttl
	}
	return ttl
}

func (p *Provider) watchNotify() {
	pubsub := p.rc.Subscribe(p.ctx, p.notifyKey())
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-p.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			parts := strings.SplitN(msg.Payload, ":", 3)
			if len(parts) == 3 {
				name, key := parts[1], parts[2]
				p.cache.Del(name + ":" + key)
			}
		}
	}
}

func (p *Provider) publishEvent(event string, name string, key string) {
	p.rc.Publish(p.ctx, p.notifyKey(), event+":"+name+":"+key)
}

func (p *Provider) hashKey(name string) string {
	return p.option.prefix + fmt.Sprintf(":hash:%s", name)
}

func (p *Provider) nameSetKey() string {
	return p.option.prefix + ":names"
}

func (p *Provider) notifyKey() string {
	return p.option.prefix + ":notify"
}
