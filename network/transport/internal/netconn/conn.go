package netconn

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/2comjie/nova/core/buffer"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/network/transport/internal/writequeue"
	"github.com/2comjie/nova/packet"
)

type Conn struct {
	conn         net.Conn
	codec        *packet.Codec
	kind         transport.Type
	secure       bool
	writes       *writequeue.Queue
	done         chan struct{}
	startOnce    sync.Once
	closeOnce    sync.Once
	notifyOnce   sync.Once
	handlerMutex sync.RWMutex
	handler      transport.Handler
}

func New(conn net.Conn, codec *packet.Codec, kind transport.Type, secure bool, writeQueue int, writeWait time.Duration) *Conn {
	if codec == nil {
		codec = packet.NewCodec(packet.DefaultMaxFrame)
	}
	writes := writequeue.New(codec, writeQueue, writeWait)
	return &Conn{
		conn:   conn,
		codec:  codec,
		kind:   kind,
		secure: secure,
		writes: writes,
		done:   writes.Done,
	}
}

func (c *Conn) Start(handler transport.Handler) error {
	if handler == nil {
		return errors.New("network: 连接Handler不能为空")
	}

	started := false
	var startErr error
	c.startOnce.Do(func() {
		started = true
		c.handlerMutex.Lock()
		c.handler = handler
		c.handlerMutex.Unlock()
		select {
		case <-c.done:
			startErr = transport.ErrClosed
			c.notifyOnce.Do(func() {
				help.SafeRun(func() {
					handler.HandleClose(c)
				})
			})
			return
		default:
		}
		help.SafeGo(c.readLoop)
		help.SafeGo(c.writeLoop)
	})
	if !started {
		return transport.ErrStarted
	}
	return startErr
}

func (c *Conn) Write(message *packet.Message) error {
	return c.WriteContext(context.Background(), message)
}

func (c *Conn) WriteContext(ctx context.Context, message *packet.Message) error {
	err := c.writes.Write(ctx, message)
	if err == transport.ErrWriteQueueFull {
		c.writes.Close()
		_ = c.conn.Close()
	}
	return err
}

func (c *Conn) readLoop() {
	defer c.Close()
	for {
		message, err := c.codec.Read(c.conn)
		if err != nil {
			return
		}

		c.handlerMutex.RLock()
		handler := c.handler
		c.handlerMutex.RUnlock()
		if handler != nil {
			help.SafeRun(func() {
				handler.HandleMessage(c, message)
			})
		}
		message.Release()

		select {
		case <-c.done:
			return
		default:
		}
	}
}

func (c *Conn) writeLoop() {
	defer c.Close()
	c.writes.Run(func(frame *buffer.Bytes, deadline time.Time) error {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		_, err := frame.WriteTo(c.conn)
		return err
	}, func() { _ = c.conn.Close() })
}

func (c *Conn) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.writes.Close()
		closeErr = c.conn.Close()

		c.handlerMutex.RLock()
		handler := c.handler
		c.handlerMutex.RUnlock()
		if handler != nil {
			c.notifyOnce.Do(func() {
				help.SafeRun(func() {
					handler.HandleClose(c)
				})
			})
		}
	})
	if errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Conn) Type() transport.Type {
	return c.kind
}

func (c *Conn) Secure() bool {
	return c.secure
}
