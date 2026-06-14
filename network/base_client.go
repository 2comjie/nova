package network

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/packet"
)

type pendingCall struct {
	ch chan packet.Message
}

type BaseClient struct {
	trans   Transport
	options *Options

	seq     atomic.Int32
	mu      sync.Mutex
	pending map[int32]*pendingCall

	pushHandlers map[int32]func(packet.Message)
	pushMu       sync.RWMutex

	chWrite chan connWrite
	ctx     context.Context
	cancel  context.CancelFunc
}

func (c *BaseClient) Init(trans Transport, opts *Options) {
	c.trans = trans
	c.options = opts
	c.pending = make(map[int32]*pendingCall)
	c.pushHandlers = make(map[int32]func(packet.Message))
	c.chWrite = make(chan connWrite, opts.WriteChSize)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	help.SafeGo(c.write)
	help.SafeGo(c.read)
	if opts.HeartbeatInterval > 0 {
		help.SafeGo(c.heartbeat)
	}
}

func (c *BaseClient) Call(route int32, data []byte) (packet.Message, error) {
	seq := c.seq.Add(1)

	buf, err := c.packData(packet.Req, route, seq, data)
	if err != nil {
		return packet.Message{}, err
	}

	ch := make(chan packet.Message, 1)
	c.mu.Lock()
	c.pending[seq] = &pendingCall{ch: ch}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
	}()

	if err = c.enqueue(buf); err != nil {
		return packet.Message{}, err
	}

	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(c.callTimeout()):
		return packet.Message{}, ErrCallTimeout
	case <-c.ctx.Done():
		return packet.Message{}, ErrConnClosed
	}
}

// Send 单向发 Req，不等响应。
func (c *BaseClient) Send(route int32, data []byte) error {
	seq := c.seq.Add(1)
	buf, err := c.packData(packet.Req, route, seq, data)
	if err != nil {
		return err
	}
	return c.enqueue(buf)
}

// RegisterPushHandler 注册服务端主动推送的处理器，按 route 分发。
func (c *BaseClient) RegisterPushHandler(route int32, fn func(packet.Message)) {
	c.pushMu.Lock()
	c.pushHandlers[route] = fn
	c.pushMu.Unlock()
}

// Close 关闭连接。
func (c *BaseClient) Close() error {
	c.cancel()
	close(c.chWrite)
	return c.trans.Close()
}

// ========= 内部方法 ==========

func (c *BaseClient) packData(msgType packet.MessageType, route, seq int32, data []byte) (buffer.Buffer, error) {
	var dataBuf buffer.Buffer
	if len(data) > 0 {
		dataBuf = buffer.MallocBytes(len(data))
		copy(dataBuf.Bytes(), data)
	}
	return c.options.Packer.PackBuffer(msgType, route, seq, dataBuf)
}

func (c *BaseClient) enqueue(buf buffer.Buffer) error {
	select {
	case c.chWrite <- connWrite{typ: connDataPacket, msg: buf}:
		return nil
	default:
		return ErrWritChFull
	}
}

func (c *BaseClient) callTimeout() time.Duration {
	if c.options.HeartbeatInterval > 0 {
		return c.options.HeartbeatInterval * 3
	}
	return 5 * time.Second
}

func (c *BaseClient) write() {
	for {
		select {
		case r, ok := <-c.chWrite:
			if !ok {
				return
			}
			err := func() error {
				defer r.msg.Release()
				_, err := r.msg.WriteTo(c.trans)
				return err
			}()
			if err != nil && !errors.Is(err, net.ErrClosed) {
				logx.Errorf("client write err %v", err)
			}
		}
	}
}

func (c *BaseClient) read() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		buff, err := c.options.Packer.ReadBuffer(c.trans)
		if err != nil {
			logx.Errorf("client read err %v", err)
			_ = c.Close()
			return
		}
		if buff == nil || buff.Len() == 0 {
			continue
		}

		msg := c.options.Packer.ToMessage(buff)
		switch msg.MessageType() {
		case packet.Pong:
			// 心跳回包，忽略
		case packet.Rsp:
			c.mu.Lock()
			p, ok := c.pending[msg.Seq()]
			c.mu.Unlock()
			if ok {
				p.ch <- msg
			}
		case packet.Push:
			c.pushMu.RLock()
			fn, ok := c.pushHandlers[msg.Route()]
			c.pushMu.RUnlock()
			if ok {
				fn(msg)
			}
		default:
			logx.Warnf("client recv unexpected msg type %d", msg.MessageType())
		}
	}
}

func (c *BaseClient) heartbeat() {
	tk := time.NewTicker(c.options.HeartbeatInterval)
	defer tk.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-tk.C:
			ping, err := c.options.Packer.PackBuffer(packet.Ping, 0, 0, nil)
			if err != nil {
				continue
			}
			select {
			case c.chWrite <- connWrite{typ: connDataPacket, msg: ping}:
			default:
				logx.Warn("client heartbeat: write chan full")
			}
		}
	}
}
