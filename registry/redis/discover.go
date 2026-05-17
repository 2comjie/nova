package redis

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Discover struct {
	option *option
	rc     redis.UniversalClient
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	instances []*endpoint.ServiceInstance
	notify    chan struct{}
}

func NewDiscover(rc redis.UniversalClient, opts ...Option) *Discover {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Discover{
		option: o,
		rc:     rc,
		ctx:    ctx,
		cancel: cancel,
		notify: make(chan struct{}, 1),
	}
	help.SafeGo(d.watchNotify)
	help.SafeGo(d.watchExpire)
	help.SafeGo(d.pollFetch)
	return d
}

func (d *Discover) List() ([]*endpoint.ServiceInstance, error) {
	d.mu.RLock()
	if d.instances != nil {
		defer d.mu.RUnlock()
		return d.instances, nil
	}
	d.mu.RUnlock()
	return d.fetchAll()
}

func (d *Discover) Next() ([]*endpoint.ServiceInstance, error) {
	select {
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	case <-d.notify:
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.instances, nil
	}
}

func (d *Discover) Close() {
	d.cancel()
}

func (d *Discover) watchNotify() {
	pubsub := d.rc.Subscribe(d.ctx, d.notifyKey())
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-d.ctx.Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			d.refresh()
		}
	}
}

func (d *Discover) watchExpire() {
	channel := "__keyevent@0__:expired"
	pubsub := d.rc.PSubscribe(d.ctx, channel)
	defer pubsub.Close()
	ch := pubsub.Channel()
	prefix := d.option.prefix + ":expire:"
	for {
		select {
		case <-d.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if strings.HasPrefix(msg.Payload, prefix) {
				instanceID := strings.TrimPrefix(msg.Payload, prefix)
				d.rc.HDel(d.ctx, d.hashKey(), instanceID)
				d.refresh()
			}
		}
	}
}

func (d *Discover) pollFetch() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	d.refresh()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.refresh()
		}
	}
}

func (d *Discover) refresh() {
	instances, err := d.fetchAll()
	if err != nil {
		zap.S().Errorf("refresh instances err %+v", err)
		return
	}
	d.mu.Lock()
	d.instances = instances
	d.mu.Unlock()
	select {
	case d.notify <- struct{}{}:
	default:
	}
}

func (d *Discover) fetchAll() ([]*endpoint.ServiceInstance, error) {
	hash, err := d.rc.HGetAll(d.ctx, d.hashKey()).Result()
	if err != nil {
		return nil, err
	}
	var instances []*endpoint.ServiceInstance
	for id, value := range hash {
		alive, err := d.rc.Exists(d.ctx, d.aliveKey(id)).Result()
		if err != nil || alive == 0 {
			d.rc.HDel(d.ctx, d.hashKey(), id)
			continue
		}
		inst := &endpoint.ServiceInstance{}
		if err := json.Unmarshal([]byte(value), inst); err != nil {
			zap.S().Errorf("unmarshal service %s err %+v", id, err)
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

func (d *Discover) hashKey() string {
	return d.option.prefix + ":hash"
}

func (d *Discover) aliveKey(instanceID string) string {
	return d.option.prefix + ":expire:" + instanceID
}

func (d *Discover) notifyKey() string {
	return d.option.prefix + ":notify"
}
