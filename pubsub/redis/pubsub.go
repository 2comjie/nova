package redisPubsub

import (
	"context"
	"encoding/json"

	"github.com/2comjie/nova/logx"
	"github.com/redis/go-redis/v9"
)

type Event[T any] struct {
	Type string `json:"type"`
	Data T      `json:"data"`
}

func Publish[T any](ctx context.Context, client redis.UniversalClient, channel string, event Event[T]) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return client.Publish(ctx, channel, payload).Err()
}

func Listen[T any](ctx context.Context, subscription *redis.PubSub, handler func(Event[T])) {
	defer subscription.Close()
	messages := subscription.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			var event Event[T]
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				logx.Errorf("pubsub: decode event failed: %v", err)
				continue
			}
			handler(event)
		}
	}
}
