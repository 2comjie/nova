package eventbus

import (
	"context"
	"encoding/json"
	"errors"
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
		Values: map[string]any{"event": string(data)},
	}).Err()
}

// Subscribe 消费 stream，group 用服务名，consumer 用实例 ID。
func (eb *EventBus) Subscribe(ctx context.Context, stream, group, consumer string, handler func([]byte)) error {
	err := eb.rc.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && !isBusyGroup(err) {
		return err
	}

	for {
		msgs, err := eb.rc.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ctx.Err()
			}
			if errors.Is(err, redis.ErrClosed) {
				return err
			}
			time.Sleep(time.Second)
			continue
		}

		for _, s := range msgs {
			for _, msg := range s.Messages {
				raw, ok := msg.Values["event"]
				if !ok {
					eb.rc.XAck(ctx, s.Stream, group, msg.ID)
					continue
				}
				handler(json.RawMessage(raw.(string)))
				eb.rc.XAck(ctx, s.Stream, group, msg.ID)
			}
		}
	}
}

func isBusyGroup(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
