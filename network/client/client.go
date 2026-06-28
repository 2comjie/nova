package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/packet"
	"go.uber.org/atomic"
)

const (
	ConnStateOpen  = 0
	ConnStateClose = 1
)

type pendingCall struct {
	ch chan packet.Message
}

type PushHandler func(message packet.Message)

func NewClient(opts ...Option) *client {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &client{
		options:      options,
		pendingCalls: make(map[int32]pendingCall),
		pushHandlers: make(map[int32]PushHandler),
		ctx:          ctx,
		cancel:       cancel,
	}
}

type client struct {
	options      *options
	pendingCalls map[int32]pendingCall
	pushHandlers map[int32]PushHandler
	nextSeq      atomic.Int32
	ctx          context.Context
	cancel       context.CancelFunc

	rw    sync.RWMutex
	state atomic.Int32
	trans Transport
}

func (c *client) Handle(trans Transport) error {
	if !c.state.CompareAndSwap(0, ConnStateOpen) {
		return errors.New("lx already connected")
	}
	c.trans = trans

	help.SafeGo(func() {
		c.read(trans)
	})
	help.SafeGo(func() {
		c.heartbeat(trans)
	})

	return nil
}

func (c *client) Call(ctx context.Context, route int32, data buffer.Buffer) (packet.Message, error) {
	if c.state.Load() != ConnStateOpen {
		return packet.Message{}, errors.New("lx closed")
	}

	seq := c.nextSeq.Add(1)

	buf := c.options.packer.PackBuffer(packet.Req, route, seq, data)

	defer buf.Release()

	ch := make(chan packet.Message, 1)

	c.rw.Lock()
	c.pendingCalls[seq] = pendingCall{ch: ch}
	c.rw.Unlock()

	defer func() {
		c.rw.Lock()
		delete(c.pendingCalls, seq)
		c.rw.Unlock()
	}()

	if _, err := buf.WriteTo(c.trans); err != nil {
		return packet.Message{}, fmt.Errorf("write: %w", err)
	}

	select {
	case <-ctx.Done():
		return packet.Message{}, ctx.Err()
	case <-c.ctx.Done():
		return packet.Message{}, errors.New("lx closed")
	case msg, ok := <-ch:
		if !ok {
			return packet.Message{}, errors.New("lx closed")
		}
		return msg, nil
	}
}

func (c *client) Send(route int32, data buffer.Buffer) error {
	if c.state.Load() != ConnStateOpen {
		return errors.New("lx closed")
	}

	buf := c.options.packer.PackBuffer(packet.Push, route, 0, data)
	defer buf.Release()

	if _, err := buf.WriteTo(c.trans); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (c *client) OnPush(route int32, handler PushHandler) {
	c.rw.Lock()
	defer c.rw.Unlock()
	c.pushHandlers[route] = handler
}

func (c *client) Close() {
	if !c.state.CompareAndSwap(ConnStateOpen, ConnStateClose) {
		return
	}

	c.cancel()
	_ = c.trans.Close()
	for _, call := range c.pendingCalls {
		close(call.ch)
	}
}

func (c *client) read(trans Transport) {
	defer func() {
		c.Close()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			buff, err := c.options.packer.ReadBuffer(trans)
			if err != nil {
				if !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
					logx.Errorf("read error: %v", err)
				}
				return
			}
			message, err := c.options.packer.ToMessage(buff)
			if err != nil {
				return
			}
			switch message.MessageType() {
			case packet.Rsp:
				seq := message.Seq()
				c.rw.RLock()
				call, ok := c.pendingCalls[seq]
				c.rw.RUnlock()
				if !ok {
					// Call 已超时清理
					return
				}
				select {
				case call.ch <- message:
				default:
				}
			case packet.Push:
				route := message.Route()
				c.rw.RLock()
				handler, ok := c.pushHandlers[route]
				c.rw.RUnlock()
				if ok {
					handler(message)
				}
			case packet.Pong:
				if c.options.onHeartBeat != nil {
					c.options.onHeartBeat()
				}
			default:
				logx.Warnf("unknown message type: %d", message.MessageType())
			}
		}
	}
}

func (c *client) heartbeat(trans Transport) {
	tk := time.NewTicker(c.options.heartBeatInterval * 2 / 3)
	defer tk.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-tk.C:
			buf := c.options.packer.PackBuffer(packet.Ping, 0, 0, nil)
			if _, err := buf.WriteTo(trans); err != nil {
				buf.Release()
				return
			}
			buf.Release()
		}
	}
}
