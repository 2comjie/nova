package redis

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/wali/core/help"
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

	mu        sync.RWMutex
	instances map[string]map[string]string
}

func NewProvider(rc redis.UniversalClient, opts ...Option) *Provider {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Provider{
		rc:      rc,
		option:  o,
		ctx:     ctx,
		cancel:  cancel,
		stopChs: make(map[string]chan struct{}),
	}
	help.SafeGo(p.watchNotify)
	help.SafeGo(p.watchExpire)
	help.SafeGo(p.pollFetch)
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
	p.mu.RLock()
	if p.instances != nil {
		nameMap, ok := p.instances[name]
		if ok {
			id, ok := nameMap[key]
			if ok {
				p.mu.RUnlock()
				return id, nil
			}
		}
	}
	p.mu.RUnlock()

	id, err := p.rc.HGet(p.ctx, p.hashKey(name), key).Result()
	if err != nil {
		return "", err
	}
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
		case _, ok := <-ch:
			if !ok {
				return
			}
			p.refresh()
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
				p.refresh()
			}
		}
	}
}

func (p *Provider) pollFetch() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	p.refresh()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.refresh()
		}
	}
}

func (p *Provider) refresh() {
	instances, err := p.fetchAll()
	if err != nil {
		zap.S().Errorf("refresh instances err %+v", err)
		return
	}
	p.mu.Lock()
	p.instances = instances
	p.mu.Unlock()
}

func (p *Provider) fetchAll() (map[string]map[string]string, error) {
	names, err := p.rc.SMembers(p.ctx, p.nameSetKey()).Result()
	if err != nil {
		return nil, err
	}
	instances := make(map[string]map[string]string)
	for _, name := range names {
		hash, err := p.rc.HGetAll(p.ctx, p.hashKey(name)).Result()
		if err != nil {
			continue
		}
		if len(hash) == 0 {
			p.rc.SRem(p.ctx, p.nameSetKey(), name)
			continue
		}
		nameMap := make(map[string]string, len(hash))
		for key, id := range hash {
			aliveKey := p.aliveKey(name, key)
			alive, err := p.rc.Exists(p.ctx, aliveKey).Result()
			if err != nil || alive == 0 {
				p.rc.Eval(p.ctx, deleteIfExpiredScript,
					[]string{p.hashKey(name), aliveKey}, key)
				continue
			}
			nameMap[key] = id
		}
		if len(nameMap) > 0 {
			instances[name] = nameMap
		}
	}
	return instances, nil
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
