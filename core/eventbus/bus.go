package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

type EventBus struct {
	rc redis.UniversalClient
}

func NewEventBus(rc redis.UniversalClient) *EventBus {
	return &EventBus{rc: rc}
}

func (eb *EventBus) Publish(ctx context.Context, stream string, data []byte) error {
	return eb.rc.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"event": string(data)},
	}).Err()
}

func (eb *EventBus) Subscribe(ctx context.Context, stream string, group string, handler func([]byte)) error {
	consumer := fmt.Sprintf("%s-%d", group, rand.Int63())

	eb.rc.XGroupCreateMkStream(ctx, stream, group, "$")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := eb.rc.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    time.Second * 5,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, redis.ErrClosed) {
				continue
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range msgs {
			for _, msg := range stream.Messages {
				raw, ok := msg.Values["event"]
				if !ok {
					eb.rc.XAck(ctx, stream.Stream, group, msg.ID)
					continue
				}
				handler(json.RawMessage(raw.(string)))
				eb.rc.XAck(ctx, stream.Stream, group, msg.ID)
			}
		}
	}
}
