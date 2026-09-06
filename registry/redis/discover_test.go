package redisRegistry

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/nova/core/endpoint"
	redisPubsub "github.com/2comjie/nova/pubsub/redis"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type discoverHook struct{ process redis.ProcessHook }

func (h discoverHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h discoverHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		if err := next(ctx, command); err != nil {
			return err
		}
		return h.process(ctx, command)
	}
}
func (h discoverHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func discoverClient(t *testing.T, db int) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: server.Addr(), DB: db})
	t.Cleanup(func() { rc.Close() })
	return rc
}

func setInstance(t *testing.T, rc *redis.Client, instance endpoint.ServiceInstance) {
	t.Helper()
	data, err := json.Marshal(instance)
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.HSet(context.Background(), "registry:hash", instance.Id, data).Err(); err != nil {
		t.Fatal(err)
	}
}

func waitSubscription(t *testing.T, rc *redis.Client, channel string, count int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		subscribers, err := rc.PubSubNumSub(ctx, channel).Result()
		if err != nil {
			t.Fatal(err)
		}
		if subscribers[channel] == count {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("channel %s subscribers = %d, want %d", channel, subscribers[channel], count)
		case <-time.After(time.Millisecond):
		}
	}
}

func nextSnapshot(t *testing.T, discover *Discover) map[string]endpoint.ServiceInstance {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	instances, err := discover.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return instances
}

func TestDiscoverReturnsSnapshots(t *testing.T) {
	rc := discoverClient(t, 0)
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-1", ServiceName: "game"})
	discover := NewDiscover(rc)
	defer discover.Close()
	notified := nextSnapshot(t, discover)
	listed, err := discover.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitSubscription(t, rc, "registry:notify", 1)
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-2", ServiceName: "game"})
	if err := redisPubsub.Publish(context.Background(), rc, "registry:notify", redisPubsub.Event[struct{}]{Type: "refresh"}); err != nil {
		t.Fatal(err)
	}
	updated := nextSnapshot(t, discover)
	if len(updated) != 2 {
		t.Fatalf("updated snapshot = %v", updated)
	}
	if len(listed) != 1 || len(notified) != 1 {
		t.Fatal("discovery update changed a previously returned snapshot")
	}
	delete(listed, "game-1")
	delete(notified, "game-1")
	if _, exists, err := discover.Get(context.Background(), "game-1"); err != nil || !exists {
		t.Fatalf("snapshot mutation changed discovery: exists=%v err=%v", exists, err)
	}
}

func TestDiscoverInitialRefreshDoesNotPublishPartialEvent(t *testing.T) {
	rc := discoverClient(t, 0)
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-1"})
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-2"})
	fetched := make(chan struct{})
	release := make(chan struct{})
	var count atomic.Int32
	rc.AddHook(discoverHook{process: func(ctx context.Context, command redis.Cmder) error {
		if command.Name() == "hgetall" && count.Add(1) == 1 {
			close(fetched)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}})
	discover := NewDiscover(rc)
	defer discover.Close()
	select {
	case <-fetched:
	case <-time.After(2 * time.Second):
		t.Fatal("initial refresh did not start")
	}
	waitSubscription(t, rc, "registry:notify", 1)
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-3"})
	if err := redisPubsub.Publish(context.Background(), rc, "registry:notify", redisPubsub.Event[struct{}]{Type: "refresh"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if instances, err := discover.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("partial initial snapshot published: instances=%v err=%v", instances, err)
	}
	close(release)
	for {
		instances := nextSnapshot(t, discover)
		if len(instances) < 2 {
			t.Fatalf("incomplete snapshot = %v", instances)
		}
		if len(instances) == 3 {
			break
		}
	}
}

func TestDiscoverNotificationsDuringRefreshLoadLatestSnapshot(t *testing.T) {
	rc := discoverClient(t, 0)
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-1", RpcHost: "initial"})
	fetched := make(chan struct{})
	release := make(chan struct{})
	var count atomic.Int32
	rc.AddHook(discoverHook{process: func(ctx context.Context, command redis.Cmder) error {
		if command.Name() == "hgetall" && count.Add(1) == 2 {
			close(fetched)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}})
	discover := NewDiscover(rc)
	defer discover.Close()
	nextSnapshot(t, discover)
	waitSubscription(t, rc, "registry:notify", 1)
	waitSubscription(t, rc, "__keyevent@0__:hexpired", 1)
	if err := redisPubsub.Publish(context.Background(), rc, "registry:notify", redisPubsub.Event[struct{}]{Type: "refresh"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fetched:
	case <-time.After(2 * time.Second):
		t.Fatal("notification did not refresh")
	}
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-1", RpcHost: "latest"})
	if err := redisPubsub.Publish(context.Background(), rc, "registry:notify", redisPubsub.Event[struct{}]{Type: "refresh"}); err != nil {
		t.Fatal(err)
	}
	if err := rc.Publish(context.Background(), "__keyevent@0__:hexpired", "registry:hash").Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for len(discover.refreshCh) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("notification was not queued during refresh")
		case <-time.After(time.Millisecond):
		}
	}
	if calls := count.Load(); calls != 2 {
		t.Fatalf("concurrent refresh started while previous fetch was pending: calls=%d", calls)
	}
	close(release)
	for {
		instances := nextSnapshot(t, discover)
		if instances["game-1"].RpcHost == "latest" {
			break
		}
	}
}

func TestDiscoverUsesConfiguredDatabaseForExpiry(t *testing.T) {
	rc := discoverClient(t, 3)
	setInstance(t, rc, endpoint.ServiceInstance{Id: "game-1"})
	discover := NewDiscover(rc)
	defer discover.Close()
	nextSnapshot(t, discover)
	waitSubscription(t, rc, "__keyevent@3__:hexpired", 1)
	if err := rc.HDel(context.Background(), "registry:hash", "game-1").Err(); err != nil {
		t.Fatal(err)
	}
	if err := rc.Publish(context.Background(), "__keyevent@3__:hexpired", "other:hash").Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := discover.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unrelated key triggered refresh: %v", err)
	}
	if err := rc.Publish(context.Background(), "__keyevent@3__:hexpired", "registry:hash").Err(); err != nil {
		t.Fatal(err)
	}
	if instances := nextSnapshot(t, discover); len(instances) != 0 {
		t.Fatalf("expired instance retained: %v", instances)
	}
}

func TestDiscoverCloseCancelsAndWaitsForWorkers(t *testing.T) {
	rc := discoverClient(t, 0)
	fetched := make(chan struct{})
	stopped := make(chan struct{})
	rc.AddHook(discoverHook{process: func(ctx context.Context, command redis.Cmder) error {
		if command.Name() == "hgetall" {
			close(fetched)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		}
		return nil
	}})
	discover := NewDiscover(rc)
	defer discover.Close()
	select {
	case <-fetched:
	case <-time.After(2 * time.Second):
		t.Fatal("initial refresh did not start")
	}
	waitSubscription(t, rc, "registry:notify", 1)
	waitSubscription(t, rc, "__keyevent@0__:hexpired", 1)
	discover.Close()
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned before refresh worker stopped")
	}
	waitSubscription(t, rc, "registry:notify", 0)
	waitSubscription(t, rc, "__keyevent@0__:hexpired", 0)
	if _, err := discover.Next(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next after Close = %v", err)
	}
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Close shut down shared Redis client: %v", err)
	}
}
