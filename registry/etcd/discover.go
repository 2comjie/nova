package etcd

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/2comjie/wali/core/endpoint"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

type Discover struct {
	client *clientv3.Client
	option *option

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	instances []*endpoint.ServiceInstance
	notify    chan struct{}
}

func NewDiscover(client *clientv3.Client, opts ...Option) *Discover {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Discover{
		client: client,
		option: o,
		ctx:    ctx,
		cancel: cancel,
		notify: make(chan struct{}, 1),
	}
	d.refresh()
	go d.watch()
	return d
}

func (d *Discover) List() ([]*endpoint.ServiceInstance, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.instances, nil
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

func (d *Discover) watch() {
	prefix := d.option.prefix + "/"
	watchCh := d.client.Watch(d.ctx, prefix, clientv3.WithPrefix())
	for {
		select {
		case <-d.ctx.Done():
			return
		case _, ok := <-watchCh:
			if !ok {
				return
			}
			d.refresh()
		}
	}
}

func (d *Discover) refresh() {
	prefix := d.option.prefix + "/"
	resp, err := d.client.Get(d.ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		zap.S().Errorf("etcd get prefix err %+v", err)
		return
	}
	instances := make([]*endpoint.ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		inst := &endpoint.ServiceInstance{}
		if err := json.Unmarshal(kv.Value, inst); err != nil {
			zap.S().Errorf("unmarshal service err %+v", err)
			continue
		}
		instances = append(instances, inst)
	}
	d.mu.Lock()
	d.instances = instances
	d.mu.Unlock()
	select {
	case d.notify <- struct{}{}:
	default:
	}
}
