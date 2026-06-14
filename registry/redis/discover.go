package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"sync"
	"time"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/redis/go-redis/v9"
)

type Discover struct {
	option *option
	rc     redis.UniversalClient
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	instances map[string]endpoint.ServiceInstance
	notify    chan struct{}
}

func (d *Discover) Get(ctx context.Context, instanceID string) (endpoint.ServiceInstance, bool, error) {
	d.mu.RLock()
	if d.instances != nil {
		defer d.mu.RUnlock()
		ins, ok := d.instances[instanceID]
		return ins, ok, nil
	}
	d.mu.RUnlock()
	m, err := d.fetchAll(ctx)
	if err != nil {
		return endpoint.ServiceInstance{}, false, err
	}
	ins, ok := m[instanceID]
	return ins, ok, nil
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

func (d *Discover) List(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	d.mu.RLock()
	if d.instances != nil {
		defer d.mu.RUnlock()
		return d.instances, nil
	}
	d.mu.RUnlock()
	m, err := d.fetchAll(ctx)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (d *Discover) Next(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	select {
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
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
		case msg, ok := <-ch:
			if !ok {
				return
			}
			d.handleEvent([]byte(msg.Payload))
		}
	}
}

func (d *Discover) watchExpire() {
	channel := "__keyevent@0__:hexpired"
	pubsub := d.rc.PSubscribe(d.ctx, channel)
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-d.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg.Payload == d.hashKey() {
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
	instances, err := d.fetchAll(d.ctx)
	if err != nil {
		logx.Errorf("refresh instances err %+v", err)
		return
	}
	d.mu.Lock()
	d.instances = instances
	d.mu.Unlock()
	d.notifyChange()
}

func (d *Discover) handleEvent(payload []byte) {
	var event UpdateEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		logx.Errorf("unmarshal event err %+v, fallback to full refresh", err)
		d.refresh()
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.instances == nil {
		d.instances = make(map[string]endpoint.ServiceInstance)
	}

	switch event.Type {
	case EventRegister, EventUpdateMeta, EventDeleteMeta:
		d.instances[event.Instance.ID] = event.Instance
	case EventDeregister:
		delete(d.instances, event.Instance.ID)
	default:
		logx.Warnf("unknown event type: %s", event.Type)
		return
	}

	d.notifyChange()
}

func (d *Discover) notifyChange() {
	select {
	case d.notify <- struct{}{}:
	default:
	}
}

func (d *Discover) fetchAll(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	_ = ctx
	hash, err := d.rc.HGetAll(d.ctx, d.hashKey()).Result()
	if err != nil {
		return nil, err
	}
	instances := make(map[string]endpoint.ServiceInstance, len(hash))
	for id, value := range hash {
		inst := endpoint.ServiceInstance{}
		if err := json.Unmarshal([]byte(value), &inst); err != nil {
			logx.Errorf("unmarshal service %s err %+v", id, err)
			continue
		}
		instances[id] = inst
	}
	return instances, nil
}

func (d *Discover) hashKey() string {
	return d.option.prefix + ":hash"
}

func (d *Discover) notifyKey() string {
	return d.option.prefix + ":notify"
}
