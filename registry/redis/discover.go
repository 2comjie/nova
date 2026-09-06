package redisRegistry

import (
	"context"
	"encoding/json"
	"maps"
	"sync"
	"time"

	"github.com/2comjie/nova/core/endpoint"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/logx"
	redisPubsub "github.com/2comjie/nova/pubsub/redis"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
)

type Discover struct {
	option *option
	rc     redis.UniversalClient
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	instances map[string]endpoint.ServiceInstance
	notify    chan struct{}
	refreshCh chan struct{}
	wait      sync.WaitGroup
}

func (d *Discover) Get(ctx context.Context, instanceId string) (endpoint.ServiceInstance, bool, error) {
	d.mu.RLock()
	if d.instances != nil {
		defer d.mu.RUnlock()
		ins, ok := d.instances[instanceId]
		return ins, ok, nil
	}
	d.mu.RUnlock()
	m, err := d.fetchAll(ctx)
	if err != nil {
		return endpoint.ServiceInstance{}, false, err
	}
	ins, ok := m[instanceId]
	return ins, ok, nil
}

func NewDiscover(rc redis.UniversalClient, opts ...Option) *Discover {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Discover{
		option:    o,
		rc:        rc,
		ctx:       ctx,
		cancel:    cancel,
		notify:    make(chan struct{}, 1),
		refreshCh: make(chan struct{}, 1),
	}
	d.wait.Add(3)
	help.SafeGo(d.watchNotify)
	help.SafeGo(d.watchExpire)
	help.SafeGo(d.pollFetch)
	return d
}

func (d *Discover) List(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	d.mu.RLock()
	if d.instances != nil {
		defer d.mu.RUnlock()
		return maps.Clone(d.instances), nil
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
		return maps.Clone(d.instances), nil
	}
}

func (d *Discover) Close() {
	d.cancel()
	d.wait.Wait()
}

func (d *Discover) watchNotify() {
	defer d.wait.Done()
	redisPubsub.Listen(d.ctx, d.rc.Subscribe(d.ctx, d.notifyKey()), func(_ redisPubsub.Event[struct{}]) {
		select {
		case d.refreshCh <- struct{}{}:
		default:
		}
	})
}

func (d *Discover) watchExpire() {
	defer d.wait.Done()
	db := 0
	if client, ok := d.rc.(*redis.Client); ok {
		db = client.Options().DB
	}
	expireChannel := "__keyevent@" + cast.ToString(db) + "__:hexpired"
	subscription := d.rc.Subscribe(d.ctx, expireChannel)
	defer subscription.Close()
	messages := subscription.Channel()
	for {
		select {
		case <-d.ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			if message.Payload != d.hashKey() {
				continue
			}
			select {
			case d.refreshCh <- struct{}{}:
			default:
			}
		}
	}
}

func (d *Discover) pollFetch() {
	defer d.wait.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	d.refresh()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-d.refreshCh:
		case <-ticker.C:
		}
		d.refresh()
	}
}

func (d *Discover) refresh() {
	instances, err := d.fetchAll(d.ctx)
	if err != nil {
		if d.ctx.Err() == nil {
			logx.Errorf("refresh instances err %+v", err)
		}
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

func (d *Discover) fetchAll(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	hash, err := d.rc.HGetAll(ctx, d.hashKey()).Result()
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
