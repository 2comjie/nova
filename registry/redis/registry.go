package redisRegistry

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	_ "embed"

	"github.com/2comjie/nova/core/endpoint"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/logx"
	"github.com/redis/go-redis/v9"
)

//go:embed register.lua
var registerScript string

//go:embed deregister.lua
var deregisterScript string

type Registry struct {
	rc     redis.UniversalClient
	option *option

	ctx    context.Context
	cancel context.CancelFunc

	stopChs map[string]chan struct{}
	rw      sync.RWMutex
}

func NewRegistry(rc redis.UniversalClient, opts ...Option) *Registry {
	o := defaultOption()
	for _, fn := range opts {
		fn(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		rc:      rc,
		option:  o,
		ctx:     ctx,
		cancel:  cancel,
		stopChs: make(map[string]chan struct{}),
	}
}

func (r *Registry) Register(serviceInstance endpoint.ServiceInstance) error {
	r.rw.Lock()
	defer r.rw.Unlock()

	logCtx := logx.WithField("service", serviceInstance.ID)
	hashKey := r.hashKey()
	ttlSeconds := int(r.option.ttl.Seconds())

	data, err := json.Marshal(serviceInstance)
	if err != nil {
		return err
	}
	if err := help.Retry(r.ctx, 3, time.Second, func() error {
		return r.rc.Eval(r.ctx, registerScript, []string{hashKey}, data, serviceInstance.ID, ttlSeconds).Err()
	}); err != nil {
		return err
	}

	r.publishEvent(UpdateEvent{Type: EventRegister, Instance: serviceInstance})

	if r.stopChs[serviceInstance.ID] == nil {
		r.stopChs[serviceInstance.ID] = make(chan struct{})
		help.SafeGo(func() {
			r.keepAlive(r.stopChs[serviceInstance.ID], serviceInstance.ID)
		})
	}
	logCtx.Debugf("register success")
	return nil
}

func (r *Registry) Deregister(instanceID string) error {
	r.rw.Lock()
	defer r.rw.Unlock()

	logCtx := logx.WithField("service", instanceID)
	hashKey := r.hashKey()

	if err := help.Retry(r.ctx, 3, time.Second, func() error {
		return r.rc.Eval(r.ctx, deregisterScript, []string{hashKey}, instanceID).Err()
	}); err != nil {
		return err
	}

	r.publishEvent(UpdateEvent{Type: EventDeregister, Instance: endpoint.ServiceInstance{ID: instanceID}})

	if r.stopChs[instanceID] != nil {
		close(r.stopChs[instanceID])
		delete(r.stopChs, instanceID)
	}
	logCtx.Debugf("deregister success")
	return nil
}

func (r *Registry) UpdateMetaData(instanceId string, meta map[string]string) error {
	r.rw.Lock()
	defer r.rw.Unlock()

	logCtx := logx.WithField("service", instanceId)
	hashKey := r.hashKey()
	ttlSeconds := int(r.option.ttl.Seconds())

	var updatedInst endpoint.ServiceInstance
	err := help.Retry(r.ctx, 3, time.Second, func() error {
		data, err := r.rc.HGet(r.ctx, hashKey, instanceId).Result()
		if err != nil {
			return err
		}

		var inst endpoint.ServiceInstance
		if err := json.Unmarshal([]byte(data), &inst); err != nil {
			return err
		}

		if inst.MetaData == nil {
			inst.MetaData = make(map[string]string)
		}
		for k, v := range meta {
			inst.MetaData[k] = v
		}

		newData, err := json.Marshal(inst)
		if err != nil {
			return err
		}

		if err := r.rc.HSet(r.ctx, hashKey, instanceId, newData).Err(); err != nil {
			return err
		}
		if err := r.rc.Do(r.ctx, "HEXPIRE", hashKey, ttlSeconds, "FIELDS", 1, instanceId).Err(); err != nil {
			return err
		}
		updatedInst = inst
		return nil
	})
	if err != nil {
		return err
	}

	r.publishEvent(UpdateEvent{Type: EventUpdateMeta, Instance: updatedInst})
	logCtx.Debugf("update meta data success")
	return nil
}

func (r *Registry) DeleteMetaData(instanceId string, keys []string) error {
	r.rw.Lock()
	defer r.rw.Unlock()

	logCtx := logx.WithField("service", instanceId)
	hashKey := r.hashKey()
	ttlSeconds := int(r.option.ttl.Seconds())

	var updatedInst endpoint.ServiceInstance
	err := help.Retry(r.ctx, 3, time.Second, func() error {
		data, err := r.rc.HGet(r.ctx, hashKey, instanceId).Result()
		if err != nil {
			return err
		}

		var inst endpoint.ServiceInstance
		if err := json.Unmarshal([]byte(data), &inst); err != nil {
			return err
		}

		for _, key := range keys {
			delete(inst.MetaData, key)
		}

		newData, err := json.Marshal(inst)
		if err != nil {
			return err
		}

		if err := r.rc.HSet(r.ctx, hashKey, instanceId, newData).Err(); err != nil {
			return err
		}
		if err := r.rc.Do(r.ctx, "HEXPIRE", hashKey, ttlSeconds, "FIELDS", 1, instanceId).Err(); err != nil {
			return err
		}
		updatedInst = inst
		return nil
	})
	if err != nil {
		return err
	}

	r.publishEvent(UpdateEvent{Type: EventDeleteMeta, Instance: updatedInst})
	logCtx.Debugf("delete meta data success")
	return nil
}

func (r *Registry) Close() {
	r.rw.Lock()
	defer r.rw.Unlock()
	r.cancel()
	for id, ch := range r.stopChs {
		close(ch)
		delete(r.stopChs, id)
	}
}

func (r *Registry) keepAlive(stopCh chan struct{}, instanceID string) {
	tk := time.NewTicker(r.option.tick)
	defer tk.Stop()
	logCtx := logx.WithField("service", instanceID)
	for {
		select {
		case <-stopCh:
			return
		case <-r.ctx.Done():
			return
		case <-tk.C:
			if err := help.Retry(r.ctx, 3, time.Second, func() error {
				return r.rc.Do(r.ctx, "HEXPIRE", r.hashKey(), int(r.option.ttl.Seconds()), "FIELDS", 1, instanceID).Err()
			}); err != nil {
				logCtx.Errorf("keep alive failed: %v", err)
			}
		}
	}
}

func (r *Registry) publishEvent(event UpdateEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		logx.Errorf("marshal event err %+v", err)
		return
	}
	r.rc.Publish(r.ctx, r.notifyKey(), data)
}

func (r *Registry) hashKey() string {
	return r.option.prefix + ":hash"
}

func (r *Registry) notifyKey() string {
	return r.option.prefix + ":notify"
}
