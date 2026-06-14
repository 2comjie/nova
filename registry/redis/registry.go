package redis

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	_ "embed"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
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

	var finalErr error
	success := help.Retry(r.ctx, 3, time.Second, func() bool {
		data, err := json.Marshal(serviceInstance)
		if err != nil {
			logCtx.Errorf("marshal service err %+v", err)
			finalErr = err
			return false
		}
		err = r.rc.Eval(r.ctx, registerScript, []string{hashKey}, data, serviceInstance.ID, ttlSeconds).Err()
		if err != nil {
			logCtx.Errorf("eval register script err %+v", err)
			finalErr = err
			return false
		}
		finalErr = nil
		return true
	})

	if !success {
		logCtx.Errorf("register failed")
		return finalErr
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

	var finalErr error
	success := help.Retry(r.ctx, 3, time.Second, func() bool {
		err := r.rc.Eval(r.ctx, deregisterScript, []string{hashKey}, instanceID).Err()
		if err != nil {
			logCtx.Errorf("eval deregister script err %+v", err)
			finalErr = err
			return false
		}
		finalErr = nil
		return true
	})

	if !success {
		logCtx.Errorf("deregister failed")
		return finalErr
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
	var finalErr error
	success := help.Retry(r.ctx, 3, time.Second, func() bool {
		data, err := r.rc.HGet(r.ctx, hashKey, instanceId).Result()
		if err != nil {
			logCtx.Errorf("hget err %+v", err)
			finalErr = err
			return false
		}

		var inst endpoint.ServiceInstance
		if err := json.Unmarshal([]byte(data), &inst); err != nil {
			logCtx.Errorf("unmarshal err %+v", err)
			finalErr = err
			return false
		}

		if inst.MetaData == nil {
			inst.MetaData = make(map[string]string)
		}
		for k, v := range meta {
			inst.MetaData[k] = v
		}

		newData, err := json.Marshal(inst)
		if err != nil {
			logCtx.Errorf("marshal err %+v", err)
			finalErr = err
			return false
		}

		if err := r.rc.HSet(r.ctx, hashKey, instanceId, newData).Err(); err != nil {
			logCtx.Errorf("hset err %+v", err)
			finalErr = err
			return false
		}
		if err := r.rc.Do(r.ctx, "HEXPIRE", hashKey, ttlSeconds, "FIELDS", 1, instanceId).Err(); err != nil {
			logCtx.Errorf("hexpire err %+v", err)
			finalErr = err
			return false
		}
		updatedInst = inst
		finalErr = nil
		return true
	})

	if !success {
		logCtx.Errorf("update meta data failed")
		return finalErr
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
	var finalErr error
	success := help.Retry(r.ctx, 3, time.Second, func() bool {
		data, err := r.rc.HGet(r.ctx, hashKey, instanceId).Result()
		if err != nil {
			logCtx.Errorf("hget err %+v", err)
			finalErr = err
			return false
		}

		var inst endpoint.ServiceInstance
		if err := json.Unmarshal([]byte(data), &inst); err != nil {
			logCtx.Errorf("unmarshal err %+v", err)
			finalErr = err
			return false
		}

		for _, key := range keys {
			delete(inst.MetaData, key)
		}

		newData, err := json.Marshal(inst)
		if err != nil {
			logCtx.Errorf("marshal err %+v", err)
			finalErr = err
			return false
		}

		if err := r.rc.HSet(r.ctx, hashKey, instanceId, newData).Err(); err != nil {
			logCtx.Errorf("hset err %+v", err)
			finalErr = err
			return false
		}
		if err := r.rc.Do(r.ctx, "HEXPIRE", hashKey, ttlSeconds, "FIELDS", 1, instanceId).Err(); err != nil {
			logCtx.Errorf("hexpire err %+v", err)
			finalErr = err
			return false
		}
		updatedInst = inst
		finalErr = nil
		return true
	})

	if !success {
		logCtx.Errorf("delete meta data failed")
		return finalErr
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
			success := help.Retry(r.ctx, 3, time.Second, func() bool {
				err := r.rc.Do(r.ctx, "HEXPIRE", r.hashKey(), int(r.option.ttl.Seconds()), "FIELDS", 1, instanceID).Err()
				if err != nil {
					logCtx.Errorf("hexpire instance field err %+v", err)
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
