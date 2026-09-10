package redisLocator

import (
	"context"
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/logx"
	redisPubsub "github.com/2comjie/nova/pubsub/redis"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
)

//go:embed bind.lua
var bindScript string

//go:embed unbind.lua
var unbindScript string

//go:embed renew.lua
var renewScript string

type bindingChange struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type Provider struct {
	rc        redis.UniversalClient
	option    *option
	ctx       context.Context
	cancel    context.CancelFunc
	wait      sync.WaitGroup
	closeOnce sync.Once

	rw            sync.Mutex
	stopChs       map[[3]string]chan struct{} // name、key、value 对应的续期任务
	onBindingLost func(name, key, value string)
	cache         *ristretto.Cache[string, string]
}

func NewProvider(rc redis.UniversalClient, opts ...Option) *Provider {
	o := defaultOption()
	for _, apply := range opts {
		apply(o)
	}
	if o.tick <= 0 || o.ttl < time.Second || o.tick >= o.ttl {
		panic("locator: require TTL >= 1s and 0 < tick < TTL")
	}
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: o.cacheMaxCost * 10, MaxCost: o.cacheMaxCost, BufferItems: 64,
	})
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Provider{
		rc: rc, option: o, ctx: ctx, cancel: cancel,
		stopChs: make(map[[3]string]chan struct{}), cache: cache,
	}
	p.wait.Add(1)
	help.SafeGo(p.watchNotify)
	return p
}

func (p *Provider) SetOnBindingLost(callback func(name, key, value string)) {
	p.onBindingLost = callback
}

func (p *Provider) Bind(ctx context.Context, name, key, value string) (string, error) {
	previous, err := p.rc.Eval(ctx, bindScript,
		[]string{p.hashKey(name), p.nameSetKey()},
		key, value, int64((p.option.ttl+time.Second-1)/time.Second), name,
	).Text()
	if err != nil {
		return "", err
	}

	binding := [3]string{name, key, value}
	stopCh := make(chan struct{})
	p.rw.Lock()
	if err := p.ctx.Err(); err != nil {
		p.rw.Unlock()
		return "", err
	}
	if previousStop := p.stopChs[binding]; previousStop != nil {
		close(previousStop)
	}
	p.stopChs[binding] = stopCh
	p.wait.Add(1)
	p.rw.Unlock()

	p.cache.Del(name + ":" + key)
	event := redisPubsub.Event[bindingChange]{Type: "bind", Data: bindingChange{Name: name, Key: key}}
	if err := redisPubsub.Publish(ctx, p.rc, p.notifyKey(), event); err != nil {
		logx.Errorf("locator: publish bind failed: %v", err)
	}
	help.SafeGo(func() { p.keepAlive(name, key, value, stopCh) })
	return previous, nil
}

func (p *Provider) Unbind(ctx context.Context, name, key, value string) error {
	binding := [3]string{name, key, value}
	p.rw.Lock()
	if stopCh := p.stopChs[binding]; stopCh != nil {
		close(stopCh)
		delete(p.stopChs, binding)
	}
	p.rw.Unlock()

	result, err := p.rc.Eval(ctx, unbindScript,
		[]string{p.hashKey(name), p.nameSetKey()},
		key, value, name,
	).Int()
	if err != nil {
		return err
	}
	p.cache.Del(name + ":" + key)
	if result == 0 {
		return nil
	}
	event := redisPubsub.Event[bindingChange]{Type: "unbind", Data: bindingChange{Name: name, Key: key}}
	if err := redisPubsub.Publish(ctx, p.rc, p.notifyKey(), event); err != nil {
		logx.Errorf("locator: publish unbind failed: %v", err)
	}
	return nil
}

func (p *Provider) Locate(ctx context.Context, name, key string) (string, error) {
	cacheKey := name + ":" + key
	if value, ok := p.cache.Get(cacheKey); ok {
		return value, nil
	}
	value, err := p.rc.HGet(ctx, p.hashKey(name), key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	p.cache.SetWithTTL(cacheKey, value, 1, p.option.ttl/2)
	return value, nil
}

func (p *Provider) Close() {
	p.closeOnce.Do(func() {
		p.rw.Lock()
		p.cancel()
		clear(p.stopChs)
		p.rw.Unlock()
		p.wait.Wait()
		p.cache.Close()
	})
}

func (p *Provider) watchNotify() {
	defer p.wait.Done()
	redisPubsub.Listen(p.ctx, p.rc.Subscribe(p.ctx, p.notifyKey()), func(event redisPubsub.Event[bindingChange]) {
		p.cache.Del(event.Data.Name + ":" + event.Data.Key)
	})
}

func (p *Provider) keepAlive(name, key, value string, stopCh chan struct{}) {
	defer p.wait.Done()
	ticker := time.NewTicker(p.option.tick)
	defer ticker.Stop()
	ttl := int64((p.option.ttl + time.Second - 1) / time.Second)
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
		}

		renewed, err := p.rc.Eval(p.ctx, renewScript, []string{p.hashKey(name)}, key, value, ttl).Int()
		if err == nil && renewed == 1 {
			continue
		}
		binding := [3]string{name, key, value}
		p.rw.Lock()
		if p.stopChs[binding] != stopCh {
			p.rw.Unlock()
			return
		}
		delete(p.stopChs, binding)
		p.rw.Unlock()
		p.cache.Del(name + ":" + key)
		if err != nil {
			logx.Errorf("locator: renewal failed name=%s key=%s: %v", name, key, err)
		}
		if p.onBindingLost != nil {
			p.onBindingLost(name, key, value)
		}
		return
	}
}

func (p *Provider) hashKey(name string) string {
	return p.option.prefix + ":hash:" + name
}

func (p *Provider) nameSetKey() string {
	return p.option.prefix + ":names"
}

func (p *Provider) notifyKey() string {
	return p.option.prefix + ":notify"
}
