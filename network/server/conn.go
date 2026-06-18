package server

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/2comjie/wali/core/buffer"
	"go.uber.org/atomic"
)

const (
	ConnStateOpen  = 0
	ConnStateClose = 1
)

type Conn interface {
	ID() int64
	UID() string
	RemoteAddr() net.Addr
	Set(k, v string)
	Del(k string)
	Range(fn func(k, v string) bool)
	Write([]byte) error // 异步写入消息
	IsOpen() bool
}

type conn struct {
	id                int64
	uid               atomic.String
	attr              sync.Map
	trans             Transport
	lastHeartbeatTime atomic.Time
	writeCh           chan buffer.Buffer
	state             atomic.Int32
	ctx               context.Context
	cancel            context.CancelFunc
}

func newConn(trans Transport, id int64, writeCh chan buffer.Buffer) *conn {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &conn{
		id:      id,
		uid:     atomic.String{},
		attr:    sync.Map{},
		trans:   trans,
		writeCh: writeCh,
		state:   atomic.Int32{},
		ctx:     ctx,
		cancel:  cancel,
	}
	conn.lastHeartbeatTime.Store(time.Now())
	conn.state.Store(ConnStateOpen)
	return conn
}

func (c *conn) ID() int64 {
	return c.id
}
func (c *conn) UID() string {
	return c.uid.Load()
}
func (c *conn) RemoteAddr() net.Addr {
	return c.trans.RemoteAddr()
}
func (c *conn) Set(key string, v string) {
	c.attr.Store(key, v)
}
func (c *conn) Del(key string) {
	c.attr.Delete(key)
}
func (c *conn) Range(fn func(k, v string) bool) {
	c.attr.Range(func(key, value any) bool {
		return fn(key.(string), value.(string))
	})
}
func (c *conn) IsOpen() bool {
	return c.state.Load() == ConnStateOpen
}
func (c *conn) Write(data []byte) error {
	if c.state.Load() != ConnStateOpen {
		return ErrConnClosed
	}
	buf := buffer.MallocBytes(len(data))
	copy(buf.Bytes(), data)
	for {
		select {
		case <-c.ctx.Done():
			buf.Release()
			return ErrConnClosed
		case c.writeCh <- buf:
			return nil
		default:
			buf.Release()
			return ErrWriteChannelFull
		}
	}
}
