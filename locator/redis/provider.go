package redis

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/wali/core/help"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

//go:embed bind.lua
var bindScript string

//go:embed unbind.lua
var unbindScript string

//go:embed delete_if_expired.lua
var deleteIfExpiredScript string

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
		zap.S().Fatalf("create ristretto cache err %+v", err)
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
	help.SafeGo(p.watchExpire)
	return p
}

func (p *Provider) Bind(name string, key string, instanceID string) error {
	p.rw.Lock()
	defer p.rw.Unlock()

	logCtx := zap.S().With("name", name).With("key", key).With("instanceID", instanceID)
	aliveKey := p.aliveKey(name, key)
	hashKey := p.hashKey(name)
	ttlSeconds := int(p.option.ttl.Seconds())

	var finalErr error
	success := help.Retry(p.ctx, 3, time.Second, func() bool {
		err := p.rc.Eval(p.ctx, bindScript, []string{hashKey, aliveKey, p.nameSetKey()}, key, instanceID, ttlSeconds, name).Err()
		if err != nil {
			logCtx.Errorf("eval bind script err %+v", err)
			finalErr = err
			return false
		}
		finalErr = nil
		return true
	})

	if !success {
		logCtx.Errorf("bind failed")
		return finalErr
	}

	p.cache.Del(name + ":" + key)
	p.publishEvent("bind", name, key)

	stopKey := name + ":" + key
	if p.stopChs[stopKey] == nil {
		p.stopChs[stopKey] = make(chan struct{})
		help.SafeGo(func() {
			p.keepAlive(aliveKey, p.stopChs[stopKey])
		})
	}

	logCtx.Debugf("bind success")
	return nil
}

func (p *Provider) Unbind(name string, key string) error {
	p.rw.Lock()
	defer p.rw.Unlock()

	logCtx := zap.S().With("name", name).With("key", key)
	aliveKey := p.aliveKey(name, key)
	hashKey := p.hashKey(name)

	err := p.rc.Eval(p.ctx, unbindScript, []string{hashKey, aliveKey, p.nameSetKey()}, key, name).Err()
	if err != nil {
		logCtx.Errorf("eval unbind script err %+v", err)
		return err
	}

	p.cache.Del(name + ":" + key)
	p.publishEvent("unbind", name, key)

	stopKey := name + ":" + key
	if ch, ok := p.stopChs[stopKey]; ok {
		close(ch)
		delete(p.stopChs, stopKey)
	}

	logCtx.Debugf("unbind success")
	return nil
}

func (p *Provider) Locate(name string, key string) (string, error) {
	locKey := name + ":" + key

	if id, ok := p.cache.Get(locKey); ok {
		return id, nil
	}

	id, err := p.rc.HGet(p.ctx, p.hashKey(name), key).Result()
	if err != nil {
		return "", err
	}

	p.cache.SetWithTTL(locKey, id, 1, p.option.ttl*3)
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

func (p *Provider) keepAlive(aliveKey string, stopCh chan struct{}) {
	tk := time.NewTicker(p.option.tick)
	defer tk.Stop()
	logCtx := zap.S().With("aliveKey", aliveKey)
	for {
		select {
		case <-stopCh:
			return
		case <-p.ctx.Done():
			return
		case <-tk.C:
			success := help.Retry(p.ctx, 3, time.Second, func() bool {
				err := p.rc.Expire(p.ctx, aliveKey, p.option.ttl).Err()
				if err != nil {
					logCtx.Errorf("expire alive key err %+v", err)
					return false
				}
				return true
			})
			if !success {
				logCtx.Errorf("keep alive failed")
			}
		}
	}
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

func (p *Provider) watchExpire() {
	channel := "__keyevent@0__:expired"
	pubsub := p.rc.PSubscribe(p.ctx, channel)
	defer pubsub.Close()
	ch := pubsub.Channel()
	prefix := p.option.prefix + ":expire:"
	for {
		select {
		case <-p.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if strings.HasPrefix(msg.Payload, prefix) {
				rest := strings.TrimPrefix(msg.Payload, prefix)
				parts := strings.SplitN(rest, ":", 2)
				if len(parts) != 2 {
					continue
				}
				name, key := parts[0], parts[1]
				p.rc.Eval(p.ctx, deleteIfExpiredScript,
					[]string{p.hashKey(name), p.aliveKey(name, key)}, key)
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

func (p *Provider) aliveKey(name string, key string) string {
	return p.option.prefix + fmt.Sprintf(":expire:%s:%s", name, key)
}

func (p *Provider) nameSetKey() string {
	return p.option.prefix + ":names"
}

func (p *Provider) notifyKey() string {
	return p.option.prefix + ":notify"
}
