package netconn

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/network/transport"
	"github.com/2comjie/wali/packet"
)

const (
	defaultWriteQueue = 256
	defaultWriteWait  = 10 * time.Second
	maxQueuedBytes    = 4 << 20
)

type writeRequest struct {
	message *packet.Message
	result  chan error
	size    int64
}

// Conn 实现 TCP、KCP 共用的流式连接读写循环。
type Conn struct {
	conn         net.Conn
	codec        *packet.Codec
	kind         transport.Type
	secure       bool
	writeWait    time.Duration
	writes       chan writeRequest
	done         chan struct{}
	startOnce    sync.Once
	closeOnce    sync.Once
	notifyOnce   sync.Once
	handlerMutex sync.RWMutex
	handler      transport.Handler
	queuedBytes  atomic.Int64
}

// New 创建一个流式连接。
func New(conn net.Conn, codec *packet.Codec, kind transport.Type, secure bool, writeQueue int, writeWait time.Duration) *Conn {
	if codec == nil {
		codec = packet.NewCodec(packet.DefaultMaxFrame)
	}
	if writeQueue <= 0 {
		writeQueue = defaultWriteQueue
	}
	if writeWait <= 0 {
		writeWait = defaultWriteWait
	}
	return &Conn{
		conn:      conn,
		codec:     codec,
		kind:      kind,
		secure:    secure,
		writeWait: writeWait,
		writes:    make(chan writeRequest, writeQueue),
		done:      make(chan struct{}),
	}
}

// Start 启动唯一的读协程和写协程。
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

// Write 将完整网络包放入单写协程，并等待本次写入完成。
func (c *Conn) Write(message *packet.Message) error {
	if message == nil {
		return packet.ErrType
	}
	request := writeRequest{
		message: message,
		result:  make(chan error, 1),
		size:    int64(packet.HeaderSize + len(message.Body)),
	}
	if c.queuedBytes.Add(request.size) > maxQueuedBytes {
		c.queuedBytes.Add(-request.size)
		_ = c.Close()
		return transport.ErrWriteQueueFull
	}
	select {
	case <-c.done:
		c.queuedBytes.Add(-request.size)
		return transport.ErrClosed
	case c.writes <- request:
	default:
		c.queuedBytes.Add(-request.size)
		_ = c.Close()
		return transport.ErrWriteQueueFull
	}

	select {
	case err := <-request.result:
		return err
	case <-c.done:
		return transport.ErrClosed
	}
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
	for {
		select {
		case <-c.done:
			return
		case request := <-c.writes:
			c.queuedBytes.Add(-request.size)
			frame, err := c.codec.Encode(request.message)
			if err == nil {
				_ = c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))
				_, err = frame.WriteTo(c.conn)
				frame.Release()
			}
			request.result <- err
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

// Close 幂等关闭连接，并且只通知一次 Handler。
func (c *Conn) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		close(c.done)
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
