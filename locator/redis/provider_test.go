package redisLocator

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) redis.UniversalClient {
	t.Helper()
	server := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return rc
}

func TestProvider_BindAndLocate(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:bind_locate"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	_, err := p.Bind(context.Background(), "game", "room_1", "instance_a")
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	id, err := p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatalf("locate failed: %v", err)
	}
	if id != "instance_a" {
		t.Fatalf("expected instance_a, got %s", id)
	}
}

func TestProvider_BindUpdate(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:bind_update"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	_, _ = p.Bind(context.Background(), "game", "room_1", "instance_a")
	_, _ = p.Bind(context.Background(), "game", "room_1", "instance_b")

	id, err := p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatalf("locate failed: %v", err)
	}
	if id != "instance_b" {
		t.Fatalf("expected instance_b after update, got %s", id)
	}
}

func TestProvider_LocateNotFound(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:locate_notfound"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	id, err := p.Locate(context.Background(), "game", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Fatalf("instance=%q, want empty", id)
	}
}

func TestProvider_Unbind(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:unbind"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	_, _ = p.Bind(context.Background(), "game", "room_1", "instance_a")

	id, err := p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatalf("locate before unbind failed: %v", err)
	}
	if id != "instance_a" {
		t.Fatalf("expected instance_a, got %s", id)
	}

	p.Unbind(context.Background(), "game", "room_1", "instance_a")

	id, err = p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Fatalf("instance=%q, want empty", id)
	}
}

func TestProvider_UnbindDoesNotDeleteNewBinding(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:unbind_new"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	if _, err := p.Bind(context.Background(), "game", "room_1", "instance_a"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Bind(context.Background(), "game", "room_1", "instance_b"); err != nil {
		t.Fatal(err)
	}
	if err := p.Unbind(context.Background(), "game", "room_1", "instance_a"); err != nil {
		t.Fatal(err)
	}

	id, err := p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "instance_b" {
		t.Fatalf("instance=%q, want instance_b", id)
	}
}

func TestProvider_MultiName(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:multi_name"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	_, _ = p.Bind(context.Background(), "game", "room_1", "inst_game_1")
	_, _ = p.Bind(context.Background(), "chat", "room_1", "inst_chat_1")

	id, err := p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatalf("locate game failed: %v", err)
	}
	if id != "inst_game_1" {
		t.Fatalf("expected inst_game_1, got %s", id)
	}

	id, err = p.Locate(context.Background(), "chat", "room_1")
	if err != nil {
		t.Fatalf("locate chat failed: %v", err)
	}
	if id != "inst_chat_1" {
		t.Fatalf("expected inst_chat_1, got %s", id)
	}
}

func TestProvider_LocateCache(t *testing.T) {
	rc := testClient(t)
	p := NewProvider(rc, WithPrefix("test:locate_cache"), WithTTL(time.Second*10), WithTick(time.Second*3))
	defer p.Close()

	_, _ = p.Bind(context.Background(), "game", "room_1", "instance_x")

	// first call: cache miss, HGet falls through
	id, err := p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatalf("locate failed: %v", err)
	}
	if id != "instance_x" {
		t.Fatalf("expected instance_x, got %s", id)
	}

	// second call: should hit ristretto cache
	id, err = p.Locate(context.Background(), "game", "room_1")
	if err != nil {
		t.Fatalf("locate (cached) failed: %v", err)
	}
	if id != "instance_x" {
		t.Fatalf("expected instance_x from cache, got %s", id)
	}
}
