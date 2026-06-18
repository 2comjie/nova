package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/packet"
	"go.uber.org/atomic"
)

type NetServer interface {
	Serve(ctx context.Context, ln Listener)
	Bind(connId int64, uid string) error
	Unbind(connId int64)
	CloseConn(connId int64, reason string)
	WriteToCid(connId int64, buf []byte) error
	WriteToUid(uid string, buf []byte) error
	ConnById(connId int64) Conn
	ConnByUid(uid string) Conn
	Range(fn func(Conn) bool)
	ConnCount() int
}

func NewNetServer(opts ...Option) NetServer {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	server := &netServer{
		options:   options,
		wg:        sync.WaitGroup{},
		rw:        sync.RWMutex{},
		conns:     make(map[int64]*conn),
		uidToConn: make(map[string]*conn),
		nextId:    atomic.Int64{},
	}
	return server
}

type netServer struct {
	options *options
	wg      sync.WaitGroup
	rw      sync.RWMutex

	conns     map[int64]*conn
	uidToConn map[string]*conn

	nextId atomic.Int64
}

func (s *netServer) Serve(ctx context.Context, ln Listener) {
	defer func() {
		for _, conn := range s.conns {
			s.CloseConn(conn.id, "server closed")
		}
		s.wg.Wait()
	}()
	for {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				trans, err := ln.Accept()
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						logx.Errorf("accept error: %s", err)
					}
					return
				}

				func() {
					s.rw.Lock()
					defer s.rw.Unlock()
					if len(s.conns) >= s.options.maxConn {
						logx.Warnf("too many conns %d than %d", len(s.conns), s.options.maxConn)
						_ = trans.Close()
						return
					}

					nextId := s.nextId.Add(1)
					writeCh := make(chan buffer.Buffer, s.options.writChSize)
					conn := newConn(trans, nextId, writeCh)
					s.conns[nextId] = conn
					s.wg.Add(1)
					help.SafeGo(func() {
						s.reader(conn)
					})
					s.wg.Add(1)
					help.SafeGo(func() {
						s.writer(conn)
					})

					help.SafeGo(func() {
						s.checkHeartbeat(conn) // 心跳检查
					})
				}()

			}
		}
	}
}

func (s *netServer) Bind(connId int64, uid string) error {
	s.rw.Lock()
	defer s.rw.Unlock()
	conn := s.conns[connId]
	if conn == nil {
		return ErrConnNotFound // 连接不存在
	}
	if conn.uid.Load() != "" && conn.uid.Load() != uid {
		return ErrConnBound // 已经绑定了其他uid
	}
	conn.uid.Store(uid)
	s.uidToConn[uid] = conn
	return nil
}
func (s *netServer) Unbind(connId int64) {
	s.rw.Lock()
	defer s.rw.Unlock()
	conn := s.conns[connId]
	if conn == nil {
		return
	}
	delete(s.uidToConn, conn.uid.Load())
	conn.uid.Store("")
}
func (s *netServer) CloseConn(connId int64, reason string) {
	s.rw.Lock()
	defer s.rw.Unlock()
	conn := s.conns[connId]
	if conn == nil {
		return
	}
	if !conn.state.CompareAndSwap(ConnStateOpen, ConnStateClose) {
		return // 已经关闭了
	}

	conn.cancel()
	close(conn.writeCh)

	_ = conn.trans.Close()
	delete(s.conns, connId)
	if conn.uid.Load() != "" {
		delete(s.uidToConn, conn.uid.Load())
	}

	if s.options.onDisconnect != nil {
		s.options.onDisconnect(conn, reason)
	}
	logx.Debugf("close conn: %d, reason: %s", connId, reason)
}
func (s *netServer) WriteToCid(connId int64, buf []byte) error {
	s.rw.RLock()
	conn := s.conns[connId]
	s.rw.RUnlock()
	if conn == nil {
		return ErrConnNotFound
	}
	return conn.Write(buf)
}
func (s *netServer) WriteToUid(uid string, buf []byte) error {
	s.rw.RLock()
	conn := s.uidToConn[uid]
	s.rw.RUnlock()
	if conn == nil {
		return ErrConnNotFound
	}
	return conn.Write(buf)
}
func (s *netServer) ConnById(connId int64) Conn {
	s.rw.RLock()
	defer s.rw.RUnlock()
	return s.conns[connId]
}
func (s *netServer) ConnByUid(uid string) Conn {
	s.rw.RLock()
	defer s.rw.RUnlock()
	return s.uidToConn[uid]
}

func (s *netServer) Range(fn func(Conn) bool) {
	s.rw.RLock()
	defer s.rw.RUnlock()
	for _, c := range s.conns {
		if !fn(c) {
			return
		}
	}
}

func (s *netServer) ConnCount() int {
	s.rw.RLock()
	defer s.rw.RUnlock()
	return len(s.conns)
}

func (s *netServer) reader(conn *conn) {
	defer s.wg.Done()

	for {
		select {
		case <-conn.ctx.Done():
			return
		default:
			ok := func() bool {
				buf, err := s.options.packer.ReadBuffer(conn.trans)
				defer func() {
					if buf != nil {
						buf.Release()
					}
				}()

				if err != nil {
					s.CloseConn(conn.id, fmt.Sprintf("read error: %s", err))
					return false
				}

				msg, err := s.options.packer.ToMessage(buf)
				if err != nil {
					s.CloseConn(conn.id, fmt.Sprintf("invalid message: %s", err))
					return false
				}
				switch msg.MessageType() {
				case packet.Req:
					if s.options.onMessage != nil {
						s.options.onMessage(conn, msg)
					}
					return true
				case packet.Ping:
					conn.lastHeartbeatTime.Store(time.Now())
					if s.options.onHeartbeat != nil {
						s.options.onHeartbeat(conn)
					}
					// 写入心跳回包
					pong := s.options.packer.PackBytes(packet.Pong, 0, 0, nil)
					_ = conn.Write(pong)
					return true
				default:
					s.CloseConn(conn.id, fmt.Sprintf("unknown message type: %d", msg.MessageType()))
					return false
				}
			}()

			if !ok {
				return
			}
		}
	}
}
func (s *netServer) writer(conn *conn) {
	defer s.wg.Done()

	for {
		select {
		case <-conn.ctx.Done():
			return
		case buf, ok := <-conn.writeCh:
			func() {
				defer func() {
					if buf != nil {
						buf.Release()
					}
				}()
				if !ok {
					return
				}
				_, err := buf.WriteTo(conn.trans)
				if err != nil {
					s.CloseConn(conn.id, fmt.Sprintf("write error: %s", err))
				}
			}()
		}
	}
}
func (s *netServer) checkHeartbeat(conn *conn) {
	tk := time.NewTicker(s.options.heartBeatInterval * 2 / 3)
	defer tk.Stop()

	for {
		select {
		case <-conn.ctx.Done():
			return
		case <-tk.C:
			if conn.lastHeartbeatTime.Load().Add(s.options.heartBeatInterval).Before(time.Now()) {
				s.CloseConn(conn.id, "heartbeat timeout") // 心跳结束
				return
			}
		}
	}
}
