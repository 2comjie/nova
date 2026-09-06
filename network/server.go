package network

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/network/protocol"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	options options
	mutex   sync.RWMutex
	byConn  map[transport.Conn]*Session
	byId    map[uint64]*Session
	byUid   map[uint64]*Session
	// Bind commits and cleanup for a UID share the same order, without holding
	// the session-map lock while running hooks or accessing Redis.
	bindings [256]sync.Mutex
	sequence uint64
	started  bool
	closed   bool
	ctx      context.Context
	cancel   context.CancelFunc
	stop     chan struct{}
	done     chan struct{}
	wait     sync.WaitGroup
}

func NewServer(opts ...Option) *Server {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	if options.auther == nil {
		panic(ErrAutherRequired)
	}
	if len(options.listeners) == 0 {
		panic(ErrListenerMissing)
	}
	if options.requestQueue <= 0 {
		panic("network: request queue must be positive")
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		options: options, byConn: make(map[transport.Conn]*Session),
		byId: make(map[uint64]*Session), byUid: make(map[uint64]*Session),
		ctx: ctx, cancel: cancel, stop: make(chan struct{}), done: make(chan struct{}),
	}

	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(err)
	}
	server.sequence = binary.LittleEndian.Uint64(seed[:]) & math.MaxInt64
	return server
}

func (s *Server) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.started {
		return errors.New("network: Server已经启动")
	}
	s.started = true
	s.wait.Add(1)
	help.SafeGo(func() {
		defer s.wait.Done()
		interval := max(10*time.Millisecond, min(time.Second, s.options.bindTimeout/2, s.options.heartbeatTimeout/2))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case now := <-ticker.C:
				var expired []*Session
				s.mutex.RLock()
				for _, session := range s.byConn {
					if !session.IsBound() {
						if now.Sub(session.acceptedAt) >= s.options.bindTimeout {
							expired = append(expired, session)
						}
					} else if now.Sub(time.Unix(0, session.heartbeatAt.Load())) >= s.options.heartbeatTimeout {
						expired = append(expired, session)
					}
				}
				s.mutex.RUnlock()
				for _, session := range expired {
					session.cancel()
					_ = session.Conn.Close()
				}
			}
		}
	})
	for _, listener := range s.options.listeners {
		s.wait.Add(1)
		help.SafeGo(func() {
			defer s.wait.Done()
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				s.mutex.Lock()
				if s.closed {
					s.mutex.Unlock()
					_ = conn.Close()
					return
				}
				ctx, cancel := context.WithCancel(s.ctx)
				s.sequence++
				session := &Session{Id: s.sequence, Conn: conn, ctx: ctx, cancel: cancel,
					queue: make(chan *packet.Message, s.options.requestQueue), acceptedAt: time.Now()}
				session.heartbeatAt.Store(session.acceptedAt.UnixNano())
				s.byConn[conn], s.byId[session.Id] = session, session
				s.wait.Add(1)
				s.mutex.Unlock()
				if err := conn.Start(s); err != nil {
					cancel()
					_ = conn.Close()
				}
				help.SafeGo(func() { s.serveSession(session) })
			}
		})
	}
	return nil
}

func (s *Server) HandleMessage(conn transport.Conn, message *packet.Message) {
	s.mutex.RLock()
	session := s.byConn[conn]
	s.mutex.RUnlock()
	if session == nil {
		_ = conn.Close()
		return
	}
	if !session.IsBound() && message.Type != packet.BindReq {
		_ = conn.Close()
		return
	}
	switch message.Type {
	case packet.BindReq, packet.Req:
		size := int64(packet.HeaderSize + len(message.Body))
		if session.queuedBytes.Add(size) > maxQueuedRequestBytes {
			session.queuedBytes.Add(-size)
			_ = conn.Close()
			return
		}
		// The transport releases its pooled frame after this callback returns.
		request := &packet.Message{Type: message.Type, Route: message.Route, Seq: message.Seq, Body: bytes.Clone(message.Body)}
		select {
		case session.queue <- request:
		default:
			session.queuedBytes.Add(-size)
			_ = conn.Close()
		}
	case packet.Ping, packet.Pong:
		session.heartbeatAt.Store(time.Now().UnixNano())
		if message.Type == packet.Ping {
			if err := conn.WriteContext(session.ctx, &packet.Message{Type: packet.Pong, Body: message.Body}); err != nil {
				_ = conn.Close()
				return
			}
		}
		if s.options.hooks.OnHeartbeat != nil {
			help.SafeRun(func() { s.options.hooks.OnHeartbeat(session) })
		}
	default:
		_ = conn.Close()
	}
}

// Closing the socket only cancels work. Cleanup runs in the session worker,
// after its current request exits, and is included in Shutdown's wait.
func (s *Server) HandleClose(conn transport.Conn) {
	s.mutex.Lock()
	session := s.byConn[conn]
	if session != nil {
		delete(s.byConn, conn)
		delete(s.byId, session.Id)
		if s.byUid[session.Uid()] == session {
			delete(s.byUid, session.Uid())
		}
	}
	s.mutex.Unlock()
	if session != nil {
		session.cancel()
	}
}

func (s *Server) serveSession(session *Session) {
	defer s.wait.Done()
	defer func() {
		session.cancel()
		_ = session.Conn.Close()
		s.HandleClose(session.Conn)
		lock := &s.bindings[session.Uid()%uint64(len(s.bindings))]
		lock.Lock()
		defer lock.Unlock()
		if s.options.hooks.OnSessionEnd != nil {
			help.SafeRun(func() { s.options.hooks.OnSessionEnd(s.ctx, session) })
		}
	}()
	if s.options.hooks.OnSessionStart != nil && help.SafeRun(func() { s.options.hooks.OnSessionStart(session) }) {
		return
	}
	for {
		select {
		case <-session.ctx.Done():
			return
		case message := <-session.queue:
			session.queuedBytes.Add(-int64(packet.HeaderSize + len(message.Body)))
			if session.ctx.Err() != nil {
				return
			}
			if help.SafeRun(func() {
				if message.Type == packet.BindReq {
					s.handleBind(session, message)
				} else {
					s.handleReq(session, message)
				}
			}) {
				return
			}
		}
	}
}

func (s *Server) handleBind(session *Session, message *packet.Message) {
	var request protocol.BindRequest
	if err := proto.Unmarshal(message.Body, &request); err != nil || len(request.Token) == 0 || len(request.Token) > s.options.maxToken {
		_ = s.writeBindResponse(session, protocol.BindCode_BIND_UNAUTHORIZED)
		_ = session.Conn.Close()
		return
	}
	uid, err := s.options.auther.Auth(request.Token)
	if err != nil || uid == 0 {
		_ = s.writeBindResponse(session, protocol.BindCode_BIND_UNAUTHORIZED)
		_ = session.Conn.Close()
		return
	}
	lock := &s.bindings[uid%uint64(len(s.bindings))]
	lock.Lock()
	defer lock.Unlock()
	if session.ctx.Err() != nil {
		return
	}
	if session.IsBound() {
		_ = session.Conn.Close()
		return
	}
	s.mutex.RLock()
	current := s.byUid[uid]
	s.mutex.RUnlock()
	if current != nil && current.Id > session.Id {
		_ = session.Conn.Close()
		return
	}
	session.uid.Store(uid)
	if s.options.hooks.OnSessionBind != nil {
		if err := s.options.hooks.OnSessionBind(session); err != nil {
			_ = s.writeBindResponse(session, protocol.BindCode_BIND_UNAUTHORIZED)
			_ = session.Conn.Close()
			return
		}
	}
	s.mutex.Lock()
	if session.ctx.Err() != nil || s.closed {
		s.mutex.Unlock()
		return
	}
	old := s.byUid[uid]
	s.byUid[uid] = session
	now := time.Now().UnixNano()
	session.boundAt.Store(now)
	session.heartbeatAt.Store(now)
	s.mutex.Unlock()
	if old != nil && old != session {
		old.cancel()
		_ = old.Conn.Close()
	}
	if err := s.writeBindResponse(session, protocol.BindCode_BIND_OK); err != nil {
		_ = session.Conn.Close()
	}
}

func (s *Server) writeBindResponse(session *Session, code protocol.BindCode) error {
	response := &protocol.BindResponse{Code: code}
	if code == protocol.BindCode_BIND_OK {
		response.HeartbeatIntervalMilli = uint32(min(s.options.heartbeat.Milliseconds(), math.MaxUint32))
	}
	body, holder, err := marshalControl(response)
	if err != nil {
		return err
	}
	defer holder.Release()
	return session.Conn.WriteContext(session.ctx, &packet.Message{Type: packet.BindRsp, Body: body})
}

func (s *Server) handleReq(session *Session, message *packet.Message) {
	body, err := decodeBody(s.options, message)
	if err != nil {
		_ = session.Conn.Close()
		return
	}
	message.Body = body
	ctx := session.ctx
	if s.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.options.requestTimeout)
		defer cancel()
	}
	if s.options.hooks.OnReq != nil {
		s.options.hooks.OnReq(&ReqContext{Context: ctx, Session: session, Request: message, NeedReply: message.Seq != 0, options: s.options})
	}
}

func (s *Server) PushUid(ctx context.Context, uid uint64, route uint32, body []byte) error {
	s.mutex.RLock()
	session := s.byUid[uid]
	s.mutex.RUnlock()
	if session == nil {
		return ErrNotBound
	}
	body, err := encodeBody(s.options, packet.Push, route, 0, body)
	if err != nil {
		return err
	}
	return session.Conn.WriteContext(ctx, &packet.Message{Type: packet.Push, Route: route, Body: body})
}

func (s *Server) KickUid(uid uint64) bool {
	s.mutex.RLock()
	session := s.byUid[uid]
	s.mutex.RUnlock()
	if session == nil {
		return false
	}
	session.cancel()
	_ = session.Conn.Close()
	return true
}

func (s *Server) KickSession(id uint64) bool {
	s.mutex.RLock()
	session := s.byId[id]
	s.mutex.RUnlock()
	if session == nil {
		return false
	}
	session.cancel()
	_ = session.Conn.Close()
	return true
}

func (s *Server) KickUidSession(uid uint64, id uint64) bool {
	s.mutex.RLock()
	session := s.byId[id]
	s.mutex.RUnlock()
	if session == nil || session.Uid() != uid {
		return false
	}
	session.cancel()
	_ = session.Conn.Close()
	return true
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mutex.Lock()
	first := !s.closed
	s.closed = true
	var sessions []*Session
	if first {
		close(s.stop)
		for _, session := range s.byConn {
			sessions = append(sessions, session)
		}
	}
	s.mutex.Unlock()
	stopCancel := context.AfterFunc(ctx, s.cancel)
	defer stopCancel()
	if first {
		for _, listener := range s.options.listeners {
			_ = listener.Close()
		}
		for _, session := range sessions {
			session.cancel()
			_ = session.Conn.Close()
		}
		go func() { s.wait.Wait(); s.cancel(); close(s.done) }()
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	}
}

func (s *Server) Broadcast(ctx context.Context, route uint32, body []byte) (uint32, error) {
	s.mutex.RLock()
	sessions := make([]*Session, 0, len(s.byUid))
	for _, session := range s.byUid {
		sessions = append(sessions, session)
	}
	s.mutex.RUnlock()
	return s.pushSessions(ctx, sessions, route, body)
}

func (s *Server) MultiPush(ctx context.Context, uidList []uint64, route uint32, body []byte) (uint32, error) {
	s.mutex.RLock()
	sessions := make([]*Session, 0, len(uidList))
	seen := make(map[uint64]bool, len(uidList))
	for _, uid := range uidList {
		if session := s.byUid[uid]; session != nil && !seen[uid] {
			seen[uid] = true
			sessions = append(sessions, session)
		}
	}
	s.mutex.RUnlock()
	return s.pushSessions(ctx, sessions, route, body)
}

func (s *Server) pushSessions(ctx context.Context, sessions []*Session, route uint32, body []byte) (uint32, error) {
	body, err := encodeBody(s.options, packet.Push, route, 0, body)
	if err != nil {
		return 0, err
	}
	var next atomic.Int64
	var success atomic.Uint32
	var wait sync.WaitGroup
	for range min(16, len(sessions)) {
		wait.Add(1)
		help.SafeGo(func() {
			defer wait.Done()
			for ctx.Err() == nil {
				index := int(next.Add(1) - 1)
				if index >= len(sessions) {
					return
				}
				session := sessions[index]
				if err := session.Conn.WriteContext(ctx, &packet.Message{Type: packet.Push, Route: route, Body: body}); err != nil {
					_ = session.Conn.Close()
				} else {
					success.Add(1)
				}
			}
		})
	}
	wait.Wait()
	return success.Load(), ctx.Err()
}
