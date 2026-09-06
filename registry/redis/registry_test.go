package redisRegistry

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/2comjie/nova/core/endpoint"
	redisPubsub "github.com/2comjie/nova/pubsub/redis"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRegistryPublishesRefreshAfterMutation(t *testing.T) {
	server := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer rc.Close()
	registry := NewRegistry(rc, WithPrefix("test:registry"), WithTick(time.Hour))
	defer registry.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	updates := rc.Subscribe(ctx, registry.notifyKey())
	defer updates.Close()
	if _, err := updates.Receive(ctx); err != nil {
		t.Fatal(err)
	}

	instance := endpoint.ServiceInstance{
		Id:          "game-1",
		ServiceName: "game",
		MetaData:    map[string]string{"region": "a", "build": "1"},
		RpcHost:     "127.0.0.1",
		RpcPort:     8000,
		Weight:      1,
	}
	for _, step := range []struct {
		name   string
		mutate func() error
		meta   map[string]string
	}{
		{
			name:   "register",
			mutate: func() error { return registry.Register(instance) },
			meta:   map[string]string{"region": "a", "build": "1"},
		},
		{
			name: "update metadata",
			mutate: func() error {
				return registry.UpdateMetaData(instance.Id, map[string]string{"region": "b", "mode": "test"})
			},
			meta: map[string]string{"region": "b", "build": "1", "mode": "test"},
		},
		{
			name:   "delete metadata",
			mutate: func() error { return registry.DeleteMetaData(instance.Id, []string{"build"}) },
			meta:   map[string]string{"region": "b", "mode": "test"},
		},
		{
			name:   "deregister",
			mutate: func() error { return registry.Deregister(instance.Id) },
		},
	} {
		t.Run(step.name, func(t *testing.T) {
			if err := step.mutate(); err != nil {
				t.Fatal(err)
			}
			message, err := updates.ReceiveMessage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var event redisPubsub.Event[struct{}]
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "refresh" {
				t.Fatalf("notification = %+v, want refresh event", event)
			}
			data, err := rc.HGet(ctx, registry.hashKey(), instance.Id).Bytes()
			if step.name == "deregister" {
				if err != redis.Nil {
					t.Fatalf("deregistered instance = %q, err = %v", data, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var stored endpoint.ServiceInstance
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			want := instance
			want.MetaData = step.meta
			if !reflect.DeepEqual(stored, want) {
				t.Fatalf("stored instance = %+v, want %+v", stored, want)
			}
		})
	}
}
