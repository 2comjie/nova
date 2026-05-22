package redis

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	_ "embed"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
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

	logCtx := zap.S().With("service", serviceInstance.ID)
	aliveKey := r.aliveKey(serviceInstance.ID)
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
		err = r.rc.Eval(r.ctx, registerScript, []string{hashKey, aliveKey}, data, serviceInstance.ID, ttlSeconds).Err()
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

	r.publishEvent("register", serviceInstance.ID)

	if r.stopChs[serviceInstance.ID] == nil {
		r.stopChs[serviceInstance.ID] = make(chan struct{})
		help.SafeGo(func() {
			r.keepAlive(r.stopChs[serviceInstance.ID], aliveKey)
		})
	}
	logCtx.Debugf("register success")
	return nil
}

func (r *Registry) Deregister(instanceID string) error {
	r.rw.Lock()
	defer r.rw.Unlock()

	logCtx := zap.S().With("service", instanceID)
	aliveKey := r.aliveKey(instanceID)
	hashKey := r.hashKey()

	var finalErr error
	success := help.Retry(r.ctx, 3, time.Second, func() bool {
		err := r.rc.Eval(r.ctx, deregisterScript, []string{hashKey, aliveKey}, instanceID).Err()
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

	r.publishEvent("deregister", instanceID)

	if r.stopChs[instanceID] != nil {
		close(r.stopChs[instanceID])
		delete(r.stopChs, instanceID)
	}
	logCtx.Debugf("deregister success")
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

func (r *Registry) keepAlive(stopCh chan struct{}, aliveKey string) {
	tk := time.NewTicker(r.option.tick)
	defer tk.Stop()
	logCtx := zap.S().With("aliveKey", aliveKey)
	for {
		select {
		case <-stopCh:
			return
		case <-r.ctx.Done():
			return
		case <-tk.C:
			success := help.Retry(r.ctx, 3, time.Second, func() bool {
				err := r.rc.Expire(r.ctx, aliveKey, r.option.ttl).Err()
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

func (r *Registry) publishEvent(event string, instanceID string) {
	r.rc.Publish(r.ctx, r.notifyKey(), event+":"+instanceID)
}

func (r *Registry) aliveKey(instanceID string) string {
	return r.option.prefix + ":expire:" + instanceID
}

func (r *Registry) hashKey() string {
	return r.option.prefix + ":hash"
}

func (r *Registry) notifyKey() string {
	return r.option.prefix + ":notify"
}
