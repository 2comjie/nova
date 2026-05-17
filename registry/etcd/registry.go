package etcd

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/2comjie/wali/core/endpoint"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

type Registry struct {
	client *clientv3.Client
	option *option

	ctx    context.Context
	cancel context.CancelFunc

	leases map[string]clientv3.LeaseID
	mu     sync.Mutex
}

func NewRegistry(client *clientv3.Client, opts ...Option) *Registry {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		client: client,
		option: o,
		ctx:    ctx,
		cancel: cancel,
		leases: make(map[string]clientv3.LeaseID),
	}
}

func (r *Registry) Register(instance endpoint.ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	logCtx := zap.S().With("service", instance.ID)

	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}

	ttlSeconds := int64(r.option.ttl / time.Second)
	grant, err := r.client.Grant(r.ctx, ttlSeconds)
	if err != nil {
		logCtx.Errorf("grant lease err %+v", err)
		return err
	}

	key := r.key(instance.ID)
	_, err = r.client.Put(r.ctx, key, string(data), clientv3.WithLease(grant.ID))
	if err != nil {
		logCtx.Errorf("put key err %+v", err)
		return err
	}

	ch, err := r.client.KeepAlive(r.ctx, grant.ID)
	if err != nil {
		logCtx.Errorf("keep alive err %+v", err)
		return err
	}
	// 消费 keepalive 响应，防止堆积
	go func() {
		for range ch {
		}
	}()

	r.leases[instance.ID] = grant.ID
	logCtx.Debugf("register success")
	return nil
}

func (r *Registry) Deregister(instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	logCtx := zap.S().With("service", instanceID)

	leaseID, ok := r.leases[instanceID]
	if ok {
		r.client.Revoke(r.ctx, leaseID)
		delete(r.leases, instanceID)
	}

	_, err := r.client.Delete(r.ctx, r.key(instanceID))
	if err != nil {
		logCtx.Errorf("delete key err %+v", err)
		return err
	}

	logCtx.Debugf("deregister success")
	return nil
}

func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, leaseID := range r.leases {
		r.client.Revoke(r.ctx, leaseID)
		delete(r.leases, id)
	}
	r.cancel()
}

func (r *Registry) key(instanceID string) string {
	return r.option.prefix + "/" + instanceID
}
