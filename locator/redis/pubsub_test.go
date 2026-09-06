package redisLocator

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestProviderPublishesJSONAndInvalidatesRemoteCache(t *testing.T) {
	rc := testClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	writer := NewProvider(rc, WithPrefix("json"), WithTTL(time.Minute), WithTick(30*time.Second))
	defer writer.Close()
	reader := NewProvider(rc, WithPrefix("json"), WithTTL(time.Minute), WithTick(30*time.Second))
	defer reader.Close()
	subscription := rc.Subscribe(ctx, "json:notify")
	defer subscription.Close()
	if _, err := subscription.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		counts, err := rc.PubSubNumSub(ctx, "json:notify").Result()
		if err != nil {
			t.Fatal(err)
		}
		if counts["json:notify"] == 3 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("notification subscribers did not become ready")
		case <-tick.C:
		}
	}

	name, key := `gate:"中文`, `room:42:"雪`
	cacheKey := name + ":" + key
	if err := rc.HSet(ctx, "json:hash:"+name, key, "old").Err(); err != nil {
		t.Fatal(err)
	}
	if value, err := reader.Locate(ctx, name, key); err != nil || value != "old" {
		t.Fatalf("initial binding=%q err=%v", value, err)
	}
	reader.cache.Wait()

	for _, eventType := range []string{"bind", "unbind"} {
		if _, ok := reader.cache.Get(cacheKey); !ok {
			t.Fatalf("cache was not populated before %s", eventType)
		}
		wantValue := "new"
		if eventType == "bind" {
			if previous, err := writer.Bind(ctx, name, key, wantValue); err != nil || previous != "old" {
				t.Fatalf("Bind previous=%q err=%v", previous, err)
			}
		} else {
			wantValue = ""
			if err := writer.Unbind(ctx, name, key, "new"); err != nil {
				t.Fatal(err)
			}
		}
		message, err := subscription.ReceiveMessage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
			t.Fatalf("invalid JSON notification %q: %v", message.Payload, err)
		}
		wantEvent := map[string]any{
			"type": eventType,
			"data": map[string]any{"name": name, "key": key},
		}
		if !reflect.DeepEqual(event, wantEvent) {
			t.Fatalf("notification=%#v, want %#v", event, wantEvent)
		}
		for {
			if _, ok := reader.cache.Get(cacheKey); !ok {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatalf("%s did not invalidate remote cache", eventType)
			case <-tick.C:
			}
		}
		if value, err := reader.Locate(ctx, name, key); err != nil || value != wantValue {
			t.Fatalf("binding after %s=%q err=%v, want %q", eventType, value, err, wantValue)
		}
		reader.cache.Wait()
	}
}

type notifyPublishHook struct{ before func() error }

func (h notifyPublishHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h notifyPublishHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h notifyPublishHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		if command.Name() == "publish" {
			if err := h.before(); err != nil {
				return err
			}
		}
		return next(ctx, command)
	}
}

func TestProviderPublishDoesNotBlockOtherBindingsOrUndoMutation(t *testing.T) {
	for _, operation := range []string{"bind", "unbind"} {
		t.Run(operation, func(t *testing.T) {
			rc := testClient(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			started, release := make(chan struct{}), make(chan struct{}, 1)
			var blockNext atomic.Bool
			rc.AddHook(notifyPublishHook{before: func() error {
				if blockNext.CompareAndSwap(true, false) {
					close(started)
					<-release
					return errors.New("notification publish failed")
				}
				return nil
			}})
			p := NewProvider(rc, WithPrefix("json"), WithTTL(time.Minute), WithTick(30*time.Second))
			defer p.Close()
			defer close(release)
			if operation == "unbind" {
				if _, err := p.Bind(ctx, "gate", "42", "node-a"); err != nil {
					t.Fatal(err)
				}
			}
			blockNext.Store(true)
			done := make(chan error, 1)
			go func() {
				if operation == "bind" {
					_, err := p.Bind(ctx, "gate", "42", "node-a")
					done <- err
				} else {
					done <- p.Unbind(ctx, "gate", "42", "node-a")
				}
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("publish did not start")
			}
			other := make(chan error, 1)
			go func() {
				_, err := p.Bind(ctx, "actor", "99", "node-b")
				other <- err
			}()
			select {
			case err := <-other:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("blocked publish held the ownership lock")
			}
			release <- struct{}{}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("successful %s returned publish error: %v", operation, err)
				}
			case <-ctx.Done():
				t.Fatal("operation did not finish after publish was released")
			}
			value, err := rc.HGet(ctx, "json:hash:gate", "42").Result()
			if operation == "bind" && (err != nil || value != "node-a") {
				t.Fatalf("binding after failed publish=%q err=%v", value, err)
			}
			if operation == "unbind" && !errors.Is(err, redis.Nil) {
				t.Fatalf("unbound key still exists after failed publish: value=%q err=%v", value, err)
			}
		})
	}
}
