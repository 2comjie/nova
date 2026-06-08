package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/help"
	innernet "github.com/2comjie/wali/core/net"
	"github.com/2comjie/wali/packet"
	"go.uber.org/zap"
)

const (
	connDataPacket int = 0
	connCloseSig   int = 1
)

type connWrite struct {
	typ int
	msg buffer.Buffer
}

type BaseConn struct {
	id      int64
	uid     atomic.Value
	attr    *connAttr
	state   atomic.Int32
	trans   Transport
	options *Options

	chWrite chan connWrite
	done    chan struct{} // write goroutine 完成信号，不可替换为 ctx
	cancel  context.CancelFunc
	ctx     context.Context

	lastHeartbeatTime atomic.Int64
}

func (c *BaseConn) Send(msg buffer.Buffer) error {
	if err := c.checkState(); err != nil {
		return err
	}
	_, err := msg.WriteTo(c.trans)
	return err
}

func (c *BaseConn) Push(msg buffer.Buffer) error {
	if err := c.checkState(); err != nil {
		return err
	}
	select {
	case c.chWrite <- connWrite{typ: connDataPacket, msg: msg}:
		return nil
	default:
		return ErrWritChFull
	}
}

func (c *BaseConn) ID() int64        { return c.id }
func (c *BaseConn) UID() string      { return c.uid.Load().(string) }
func (c *BaseConn) State() ConnState { return ConnState(c.state.Load()) }
func (c *BaseConn) Attr() Attr       { return c.attr }

func (c *BaseConn) Bind(uid string) { c.uid.Store(uid) }
func (c *BaseConn) Unbind()         { c.uid.Store("") }

func (c *BaseConn) Close(reason string, force ...bool) error {
	zap.S().Debugf("conn %d close %s", c.id, reason)
	if len(force) > 0 && force[0] {
		return c.forceClose()
	}
	return c.graceClose()
}

func (c *BaseConn) LocalIP() (string, error) {
	addr, err := c.LocalAddr()
	if err != nil {
		return "", err
	}
	return innernet.ExtractIP(addr)
}

func (c *BaseConn) LocalAddr() (net.Addr, error) {
	if err := c.checkState(); err != nil {
		return nil, err
	}
	return c.trans.LocalAddr(), nil
}

func (c *BaseConn) RemoteIP() (string, error) {
	addr, err := c.RemoteAddr()
	if err != nil {
		return "", err
	}
	return innernet.ExtractIP(addr)
}

func (c *BaseConn) RemoteAddr() (net.Addr, error) {
	if err := c.checkState(); err != nil {
		return nil, err
	}
	return c.trans.RemoteAddr(), nil
}

// ========= 内部方法 ==========

func (c *BaseConn) init(mgr *BaseConnMgr, id int64, trans Transport) {
	c.id = id
	c.uid.Store("")
	c.attr = &connAttr{}
	c.state.Store(int32(ConnOpened))
	c.trans = trans
	c.options = mgr.options
	c.chWrite = make(chan connWrite, c.options.WriteChSize)
	c.done = make(chan struct{})
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.lastHeartbeatTime.Store(time.Now().UnixNano())

	help.SafeGo(c.write)
	help.SafeGo(c.read)
	if c.options.HeartbeatInterval > 0 {
		help.SafeGo(c.checkHealth)
	}
	if c.options.OnConnect != nil {
		c.options.OnConnect(c)
	}
}

func (c *BaseConn) checkHealth() {
	tk := time.NewTicker(c.options.HeartbeatCheckInterval)
	defer tk.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-tk.C:
			nowUnixNano := time.Now().UnixNano()
			if c.lastHeartbeatTime.Load()+c.options.HeartbeatInterval.Nanoseconds() < nowUnixNano {
				_ = c.Close("conn expire", true)
				return
			}
		}
	}
}

func (c *BaseConn) checkState() error {
	switch c.State() {
	case ConnHanged:
		return ErrConnHanged
	case ConnClosed:
		return ErrConnClosed
	default:
		return nil
	}
}

func (c *BaseConn) isClosed() bool {
	return c.State() == ConnClosed
}

func (c *BaseConn) read() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		buff, err := c.options.Packer.ReadBuffer(c.trans)
		if err != nil {
			_ = c.Close(fmt.Sprintf("packet err %v", err), true)
			return
		}

		switch c.State() {
		case ConnHanged, ConnClosed:
			continue
		case ConnOpened:
			if buff == nil || buff.Len() == 0 {
				continue
			}
			msg := c.options.Packer.ToMessage(buff)
			switch msg.MessageType() {
			case packet.Ping:
				if c.options.HeartbeatInterval > 0 {
					c.lastHeartbeatTime.Store(time.Now().UnixNano())
				}
				if pong, err := c.options.Packer.PackBuffer(packet.Pong, 0, 0, nil); err == nil {
					_ = c.Push(pong)
				}
				if c.options.OnHeartbeat != nil {
					c.options.OnHeartbeat(c)
				}
			case packet.Req:
				if c.options.OnMessage != nil {
					c.options.OnMessage(c, msg)
				}
			}
		default:
			_ = c.Close("unknown conn state", true)
			zap.S().Errorf("unknown conn state")
			return
		}
	}
}

func (c *BaseConn) write() {
	for {
		select {
		case r, ok := <-c.chWrite:
			if !ok {
				return
			}
			if r.typ == connCloseSig {
				c.done <- struct{}{}
				return
			}
			if c.isClosed() {
				return
			}
			err := func() error {
				defer r.msg.Release()
				_, err := r.msg.WriteTo(c.trans)
				return err
			}()
			if err != nil && !errors.Is(err, net.ErrClosed) {
				zap.S().Errorf("write packet err %v", err)
			}
		}
	}
}

func (c *BaseConn) forceClose() error {
	if !c.state.CompareAndSwap(int32(ConnOpened), int32(ConnClosed)) {
		if !c.state.CompareAndSwap(int32(ConnHanged), int32(ConnClosed)) {
			return ErrConnClosed
		}
	}
	return c.doClose()
}

func (c *BaseConn) graceClose() error {
	if !c.state.CompareAndSwap(int32(ConnOpened), int32(ConnHanged)) {
		return ErrConnNotOpen
	}
	c.chWrite <- connWrite{typ: connCloseSig}
	<-c.done // 等 write goroutine 排空队列后回报
	if !c.state.CompareAndSwap(int32(ConnHanged), int32(ConnClosed)) {
		return ErrConnNotHanged
	}
	return c.doClose()
}

func (c *BaseConn) doClose() error {
	c.cancel()       // 广播取消，read/checkHealth goroutine 退出
	close(c.chWrite) // write goroutine 退出（forceClose 路径）
	close(c.done)

	err := c.trans.Close()
	if c.options.OnDisconnect != nil {
		c.options.OnDisconnect(c)
	}
	return err
}
