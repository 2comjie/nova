package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// Subscribe 消费 stream，consumer 固定由调用方传入（通常用服务实例ID），
// 保证重启后能认领未 ACK 的历史消息。
func (eb *EventBus) Subscribe(ctx context.Context, stream, group, consumer string, handler func([]byte)) error {
	// 创建消费者组，忽略 BUSYGROUP（已存在），其他错误返回
	err := eb.rc.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && !isBusyGroup(err) {
		return fmt.Errorf("create consumer group: %w", err)
	}

	// 启动时先认领并处理本 consumer 历史未 ACK 的消息
	if err := eb.reclaimPending(ctx, stream, group, consumer, handler); err != nil {
		return err
	}

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
				eb.handleMsg(ctx, s.Stream, group, msg, handler)
			}
		}
	}
}

// 认领并处理本 consumer 重启前未 ACK 的消息。
func (eb *EventBus) reclaimPending(ctx context.Context, stream, group, consumer string, handler func([]byte)) error {
	for {
		// XAUTOCLAIM 把空闲超过 0ms 的 PEL 消息转移给本 consumer
		msgs, nextID, err := eb.rc.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   stream,
			Group:    group,
			Consumer: consumer,
			MinIdle:  0,
			Start:    "0-0",
			Count:    100,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return fmt.Errorf("reclaim pending: %w", err)
		}

		for _, msg := range msgs {
			eb.handleMsg(ctx, stream, group, msg, handler)
		}

		// nextID 为 "0-0" 表示 PEL 已全部处理完
		if nextID == "0-0" {
			return nil
		}
	}
}

func (eb *EventBus) handleMsg(ctx context.Context, stream, group string, msg redis.XMessage, handler func([]byte)) {
	raw, ok := msg.Values["event"]
	if !ok {
		eb.rc.XAck(ctx, stream, group, msg.ID)
		return
	}
	handler(json.RawMessage(raw.(string)))
	eb.rc.XAck(ctx, stream, group, msg.ID)
}

func isBusyGroup(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
