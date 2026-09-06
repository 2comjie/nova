package writequeue

import (
	"context"
	"sync"
	"time"

	"github.com/2comjie/nova/core/buffer"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
)

type request struct {
	ctx    context.Context
	frame  *buffer.Bytes
	result chan error
}

// Queue owns encoded frames, so callers can reuse their message when Write returns.
type Queue struct {
	Done chan struct{}

	codec       *packet.Codec
	writes      chan request
	writeWait   time.Duration
	mutex       sync.Mutex
	queuedBytes int
}

func New(codec *packet.Codec, size int, writeWait time.Duration) *Queue {
	if size <= 0 {
		size = 256
	}
	if writeWait <= 0 {
		writeWait = 10 * time.Second
	}
	return &Queue{Done: make(chan struct{}), codec: codec, writes: make(chan request, size), writeWait: writeWait}
}

func (q *Queue) Write(ctx context.Context, message *packet.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	frame, err := q.codec.Encode(message)
	if err != nil {
		return err
	}
	value := request{ctx: ctx, frame: frame, result: make(chan error, 1)}
	q.mutex.Lock()
	select {
	case <-q.Done:
		err = transport.ErrClosed
	default:
		if q.queuedBytes+frame.Len() > 4<<20 {
			err = transport.ErrWriteQueueFull
		} else {
			select {
			case q.writes <- value:
				q.queuedBytes += frame.Len()
			default:
				err = transport.ErrWriteQueueFull
			}
		}
	}
	q.mutex.Unlock()
	if err != nil {
		frame.Release()
		return err
	}
	select {
	case err := <-value.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-q.Done:
		if err := ctx.Err(); err != nil {
			return err
		}
		return transport.ErrClosed
	}
}

func (q *Queue) Run(write func(*buffer.Bytes, time.Time) error, interrupt func()) {
	for {
		select {
		case <-q.Done:
			return
		case value := <-q.writes:
			q.mutex.Lock()
			q.queuedBytes -= value.frame.Len()
			q.mutex.Unlock()
			if err := value.ctx.Err(); err != nil {
				value.frame.Release()
				value.result <- err
				continue
			}
			deadline := time.Now().Add(q.writeWait)
			ctxDeadline, hasDeadline := value.ctx.Deadline()
			if hasDeadline && ctxDeadline.Before(deadline) {
				deadline = ctxDeadline
			}
			// A canceled in-flight frame may be partial; close the connection instead of reusing it.
			interrupted := make(chan struct{})
			stop := context.AfterFunc(value.ctx, func() {
				interrupt()
				close(interrupted)
			})
			err := write(value.frame, deadline)
			if !stop() {
				<-interrupted
			}
			value.frame.Release()
			if ctxErr := value.ctx.Err(); ctxErr != nil {
				err = ctxErr
			} else if err != nil && hasDeadline && !time.Now().Before(ctxDeadline) {
				err = context.DeadlineExceeded
			}
			value.result <- err
			if err != nil {
				return
			}
		}
	}
}

func (q *Queue) Close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	select {
	case <-q.Done:
		return
	default:
		close(q.Done)
	}
	for {
		select {
		case value := <-q.writes:
			q.queuedBytes -= value.frame.Len()
			value.frame.Release()
		default:
			return
		}
	}
}
