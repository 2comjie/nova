package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/2comjie/wali/core/endpoint"
	registryRedis "github.com/2comjie/wali/registry/redis"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient() redis.UniversalClient {
	return redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{"127.0.0.1:6379"},
	})
}

func TestRegistry(t *testing.T) {
	rc := NewRedisClient()
	ctx := context.Background()
	if err := rc.Ping(ctx).Err(); err != nil {
		t.Skip("redis not available:", err)
	}

	reg := registryRedis.NewRegistry(rc,
		registryRedis.WithPrefix("test_registry"),
		registryRedis.WithTTL(time.Second*5),
		registryRedis.WithTick(time.Second*2),
	)
	defer reg.Close()

	inst := endpoint.ServiceInstance{
		ID:     "svc-1",
		Weight: 10,
		Status: endpoint.Work,
		RpcServices: map[string]endpoint.RpcService{
			"grpc": {Host: "127.0.0.1", Port: 8080, Name: "test-svc"},
		},
	}

	if err := reg.Register(inst); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 验证 hash 中有数据
	val, err := rc.HGet(ctx, "test_registry:hash", "svc-1").Result()
	if err != nil {
		t.Fatalf("hget failed: %v", err)
	}
	if val == "" {
		t.Fatal("expected non-empty value in hash")
	}

	// 验证 alive key 存在
	exists, err := rc.Exists(ctx, "test_registry:expire:svc-1").Result()
	if err != nil {
		t.Fatalf("exists failed: %v", err)
	}
	if exists == 0 {
		t.Fatal("expected alive key to exist")
	}

	// deregister
	if err := reg.Deregister("svc-1"); err != nil {
		t.Fatalf("deregister failed: %v", err)
	}

	exists, err = rc.Exists(ctx, "test_registry:expire:svc-1").Result()
	if err != nil {
		t.Fatalf("exists failed: %v", err)
	}
	if exists != 0 {
		t.Fatal("expected alive key to be deleted")
	}

	val, err = rc.HGet(ctx, "test_registry:hash", "svc-1").Result()
	if err == nil && val != "" {
		t.Fatal("expected hash field to be deleted")
	}
}

func TestDiscover(t *testing.T) {
	rc := NewRedisClient()
	ctx := context.Background()
	if err := rc.Ping(ctx).Err(); err != nil {
		t.Skip("redis not available:", err)
	}

	// 清理
	rc.Del(ctx, "test_discover:hash", "test_discover:expire:svc-a", "test_discover:expire:svc-b")

	reg := registryRedis.NewRegistry(rc,
		registryRedis.WithPrefix("test_discover"),
		registryRedis.WithTTL(time.Second*5),
		registryRedis.WithTick(time.Second*2),
	)
	defer reg.Close()

	disc := registryRedis.NewDiscover(rc,
		registryRedis.WithPrefix("test_discover"),
	)
	defer disc.Close()

	// 等 discover 初始化完成
	time.Sleep(time.Millisecond * 200)

	instA := endpoint.ServiceInstance{
		ID:     "svc-a",
		Weight: 5,
		Status: endpoint.Work,
		RpcServices: map[string]endpoint.RpcService{
			"grpc": {Host: "10.0.0.1", Port: 9000, Name: "svc-a"},
		},
	}
	instB := endpoint.ServiceInstance{
		ID:     "svc-b",
		Weight: 10,
		Status: endpoint.Work,
		RpcServices: map[string]endpoint.RpcService{
			"grpc": {Host: "10.0.0.2", Port: 9000, Name: "svc-b"},
		},
	}

	if err := reg.Register(instA); err != nil {
		t.Fatalf("register svc-a failed: %v", err)
	}
	if err := reg.Register(instB); err != nil {
		t.Fatalf("register svc-b failed: %v", err)
	}

	// 等通知到达
	time.Sleep(time.Millisecond * 500)

	instances, err := disc.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}

	// deregister 一个
	if err := reg.Deregister("svc-a"); err != nil {
		t.Fatalf("deregister svc-a failed: %v", err)
	}

	time.Sleep(time.Millisecond * 500)

	instances, err = disc.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance after deregister, got %d", len(instances))
	}
	if instances[0].ID != "svc-b" {
		t.Fatalf("expected svc-b, got %s", instances[0].ID)
	}
}
