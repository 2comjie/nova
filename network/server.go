package network

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/network/protocol"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	options options
	manager *sessionManager
	started atomic.Bool
	closed  atomic.Bool
	wait    sync.WaitGroup
}

func NewServer(opts ...Option) (*Server, error) {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	if options.auther == nil {
		return nil, ErrAutherRequired
	}
	if len(options.listeners) == 0 {
		return nil, ErrListenerMissing
	}

	return &Server{
		options: options,
		manager: newSessionManager(options),
	}, nil
}

func (s *Server) Start() error {
	if s.closed.Load() {
		return ErrClosed
	}
	if !s.started.CompareAndSwap(false, true) {
		return errors.New("network: Server已经启动")
	}

	s.manager.Start()
	for _, listener := range s.options.listeners {
		s.wait.Add(1)
		help.SafeGo(func() {
			defer s.wait.Done()
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				if s.closed.Load() {
					_ = conn.Close()
					return
				}
				s.manager.Add(conn)
				if err := conn.Start(s); err != nil {
					s.manager.Remove(conn)
					_ = conn.Close()
				}
			}
		})
	}
	return nil
}

func (s *Server) HandleMessage(conn transport.Conn, message *packet.Message) {
	session := s.manager.ByConn(conn)
	if session == nil {
		_ = conn.Close()
		return
	}

	if !session.IsBound() {
		if message.Type != packet.BindReq {
			_ = conn.Close()
			return
		}
		s.handleBind(session, message)
		return
	}

	switch message.Type {
	case packet.Req:
		s.handleReq(session, message)
	case packet.Ping:
		s.manager.Heartbeat(session)
		if err := conn.Write(&packet.Message{
			Type: packet.Pong,
			Body: message.Body,
		}); err != nil {
			_ = conn.Close()
		}
	case packet.Pong:
		s.manager.Heartbeat(session)
	default:
		_ = conn.Close()
	}
}

func (s *Server) HandleClose(conn transport.Conn) {
	s.manager.Remove(conn)
}

func (s *Server) handleBind(session *Session, message *packet.Message) {
	var request protocol.BindRequest
	if err := proto.Unmarshal(message.Body, &request); err != nil ||
		len(request.Token) == 0 || len(request.Token) > s.options.maxToken {
		s.writeBindResponse(session, protocol.BindCode_BIND_UNAUTHORIZED)
		_ = session.Conn.Close()
		return
	}

	if err := s.manager.Bind(session, request.Token); err != nil {
		s.writeBindResponse(session, protocol.BindCode_BIND_UNAUTHORIZED)
		_ = session.Conn.Close()
		return
	}
	if s.options.hooks.OnSessionBind != nil {
		var bindErr error
		completed := false
		help.SafeRun(func() {
			bindErr = s.options.hooks.OnSessionBind(session)
			completed = true
		})
		if !completed || bindErr != nil {
			s.writeBindResponse(session, protocol.BindCode_BIND_UNAUTHORIZED)
			_ = session.Conn.Close()
			return
		}
	}
	if err := s.writeBindResponse(session, protocol.BindCode_BIND_OK); err != nil {
		_ = session.Conn.Close()
		return
	}
}

func (s *Server) writeBindResponse(session *Session, code protocol.BindCode) error {
	response := &protocol.BindResponse{Code: code}
	if code == protocol.BindCode_BIND_OK {
		milliseconds := s.options.heartbeat.Milliseconds()
		if milliseconds > math.MaxUint32 {
			milliseconds = math.MaxUint32
		}
		response.HeartbeatIntervalMilli = uint32(milliseconds)
	}
	body, holder, err := marshalControl(response)
	if err != nil {
		return err
	}
	defer holder.Release()
	return session.Conn.Write(&packet.Message{
		Type: packet.BindRsp,
		Body: body,
	})
}

func (s *Server) handleReq(session *Session, message *packet.Message) {
	body, err := decodeBody(s.options, message)
	if err != nil {
		_ = session.Conn.Close()
		return
	}

	request := &packet.Message{
		Type:  message.Type,
		Route: message.Route,
		Seq:   message.Seq,
		Body:  body,
	}
	if s.options.hooks.OnReq != nil {
		help.SafeRun(func() {
			s.options.hooks.OnReq(&ReqContext{
				Session:   session,
				Request:   request,
				NeedReply: request.Seq != 0,
				options:   s.options,
			})
		})
	}
}

func (s *Server) PushUID(ctx context.Context, uid uint64, route uint32, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	session := s.manager.ByUID(uid)
	if session == nil {
		return ErrNotBound
	}

	body, err := encodeBody(s.options, packet.Push, route, 0, body)
	if err != nil {
		return err
	}
	return session.Conn.Write(&packet.Message{
		Type:  packet.Push,
		Route: route,
		Body:  body,
	})
}

func (s *Server) KickUID(uid uint64) bool {
	return s.manager.KickUID(uid)
}

func (s *Server) KickSession(id uint64) bool {
	return s.manager.KickSession(id)
}

func (s *Server) KickUIDSession(uid uint64, id uint64) bool {
	return s.manager.KickUIDSession(uid, id)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	for _, listener := range s.options.listeners {
		_ = listener.Close()
	}
	s.manager.Close()

	waitDone := make(chan struct{})
	help.SafeGo(func() {
		s.wait.Wait()
		close(waitDone)
	})
	select {
	case <-waitDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) Broadcast(ctx context.Context, route uint32, body []byte) (uint32, error) {
	sessions := s.manager.BoundSessions()
	return s.pushSessions(ctx, sessions, route, body)
}

func (s *Server) MultiPush(ctx context.Context, uidList []uint64, route uint32, body []byte) (uint32, error) {
	sessions := s.manager.ByUIDs(uidList)
	return s.pushSessions(ctx, sessions, route, body)
}

func (s *Server) pushSessions(ctx context.Context, sessions []*Session, route uint32, body []byte) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(sessions) == 0 {
		return 0, nil
	}

	encodedBody, err := encodeBody(s.options, packet.Push, route, 0, body)
	if err != nil {
		return 0, err
	}

	workerCount := min(16, len(sessions))

	var (
		nextIndex atomic.Int64
		success   atomic.Uint32
		wait      sync.WaitGroup
	)

	wait.Add(workerCount)

	for workerID := 0; workerID < workerCount; workerID++ {
		help.SafeGo(func() {
			defer wait.Done()

			for {
				if ctx.Err() != nil {
					return
				}

				index := int(nextIndex.Add(1) - 1)
				if index >= len(sessions) {
					return
				}

				session := sessions[index]
				err := session.Conn.Write(&packet.Message{
					Type:  packet.Push,
					Route: route,
					Body:  encodedBody,
				})
				if err != nil {
					_ = session.Conn.Close()
					continue
				}

				success.Add(1)
			}
		})
	}

	wait.Wait()

	if err := ctx.Err(); err != nil {
		return success.Load(), err
	}
	return success.Load(), nil
}
