package redisPubsub_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	redisPubsub "github.com/2comjie/nova/pubsub/redis"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

func TestListenMessages(t *testing.T) {
	type binding struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	for _, pattern := range []bool{false, true} {
		name := "channels"
		if pattern {
			name = "pattern"
		}
		t.Run(name, func(t *testing.T) {
			_, client := testClient(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var subscription *redis.PubSub
			acks := 2
			if pattern {
				subscription = client.PSubscribe(ctx, "events:*")
				acks = 1
			} else {
				subscription = client.Subscribe(ctx, "events:first", "events:second")
			}
			defer subscription.Close()
			for range acks {
				if _, err := subscription.Receive(ctx); err != nil {
					t.Fatal(err)
				}
			}
			received := make(chan redisPubsub.Event[binding], 2)
			done := make(chan struct{})
			go func() {
				defer close(done)
				redisPubsub.Listen(ctx, subscription, func(event redisPubsub.Event[binding]) { received <- event })
			}()

			for channel, want := range map[string]redisPubsub.Event[binding]{
				"events:first":  {Type: "bind", Data: binding{Name: "gate:大厅", Key: "玩家:\"42\""}},
				"events:second": {Type: "unbind", Data: binding{Name: "node:游戏", Key: "玩家:43"}},
			} {
				if err := redisPubsub.Publish(ctx, client, channel, want); err != nil {
					t.Fatal(err)
				}
				select {
				case got := <-received:
					if got != want {
						t.Fatalf("event = %+v, want %+v", got, want)
					}
				case <-ctx.Done():
					t.Fatal("message was not delivered")
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Listen did not stop after cancellation")
			}
		})
	}
}

func TestPublishJSON(t *testing.T) {
	_, client := testClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	subscription := client.Subscribe(ctx, "events")
	defer subscription.Close()
	if _, err := subscription.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	event := redisPubsub.Event[string]{Type: "bind", Data: "actor:玩家 \"hello\" 👋"}
	if err := redisPubsub.Publish(ctx, client, "events", event); err != nil {
		t.Fatal(err)
	}
	message, err := subscription.ReceiveMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"type":"bind","data":"actor:玩家 \"hello\" 👋"}`; message.Payload != want {
		t.Fatalf("payload = %q, want %q", message.Payload, want)
	}
}

func TestPublishReturnsMarshalErrorWithoutPublishing(t *testing.T) {
	server, client := testClient(t)
	err := redisPubsub.Publish(t.Context(), client, "events", redisPubsub.Event[chan int]{Type: "invalid", Data: make(chan int)})
	if _, ok := err.(*json.UnsupportedTypeError); !ok {
		t.Fatalf("error = %T %v, want direct JSON unsupported type error", err, err)
	}
	if got := server.CommandCount(); got != 0 {
		t.Fatalf("Redis commands = %d, want 0", got)
	}
}

func TestListenSkipsMalformedJSON(t *testing.T) {
	server, client := testClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	subscription := client.Subscribe(ctx, "events")
	defer subscription.Close()
	if _, err := subscription.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	received := make(chan redisPubsub.Event[string], 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		redisPubsub.Listen(ctx, subscription, func(event redisPubsub.Event[string]) { received <- event })
	}()
	server.Publish("events", `{"type":"bind","data":`)
	server.Publish("events", `{"type":"unbind","data":"next"}`)
	select {
	case got := <-received:
		if want := (redisPubsub.Event[string]{Type: "unbind", Data: "next"}); got != want {
			t.Fatalf("event = %+v, want %+v", got, want)
		}
	case <-ctx.Done():
		t.Fatal("valid event after malformed JSON was not delivered")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not stop after cancellation")
	}
}

func TestListenCallsHandlerSerially(t *testing.T) {
	server, client := testClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	subscription := client.Subscribe(ctx, "events")
	defer subscription.Close()
	if _, err := subscription.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 3)
	release := make(chan struct{}, 1)
	defer close(release)
	done := make(chan struct{})
	go func() {
		defer close(done)
		redisPubsub.Listen(ctx, subscription, func(event redisPubsub.Event[string]) {
			received <- event.Data
			if event.Data == "first" {
				<-release
			}
		})
	}()
	server.Publish("events", `{"type":"item","data":"first"}`)
	select {
	case <-received:
	case <-ctx.Done():
		t.Fatal("first callback did not start")
	}
	server.Publish("events", `{"type":"item","data":"second"}`)
	server.Publish("events", `{"type":"item","data":"third"}`)
	select {
	case got := <-received:
		t.Fatalf("callback %q ran before the first callback returned", got)
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	for _, want := range []string{"second", "third"} {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("payload = %q, want %q", got, want)
			}
		case <-ctx.Done():
			t.Fatal("remaining callback did not run")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not stop after cancellation")
	}
}

func TestListenCancelWaitsForHandlerAndClosesSubscription(t *testing.T) {
	server, client := testClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	subscription := client.Subscribe(ctx, "events")
	defer subscription.Close()
	if _, err := subscription.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{}, 1)
	defer close(release)
	done := make(chan struct{})
	go func() {
		defer close(done)
		redisPubsub.Listen(ctx, subscription, func(redisPubsub.Event[string]) {
			close(started)
			<-release
		})
	}()
	server.Publish("events", `{"type":"item","data":"first"}`)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not start")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("Listen returned before the callback finished")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not stop after the callback finished")
	}
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for server.PubSubNumSub("events")["events"] != 0 || server.CurrentConnectionCount() != 0 {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatal("subscription was not released")
		}
	}
}

func TestListenReturnsAfterSubscriptionClose(t *testing.T) {
	_, client := testClient(t)
	ctx := t.Context()
	subscription := client.Subscribe(ctx, "events")
	if _, err := subscription.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		redisPubsub.Listen(ctx, subscription, func(redisPubsub.Event[string]) {})
	}()
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not stop after the subscription was closed")
	}
}
