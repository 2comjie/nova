package rpc

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network/transport"
	netTcp "github.com/2comjie/nova/network/transport/tcp"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

type Handler func(context.Context, []byte) (proto.Message, *Error)
type ServerOption func(*Server)

func WithMaxConcurrent(limit int) ServerOption {
	return func(s *Server) { s.slots = make(chan struct{}, limit) }
}

func WithRequestTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) { s.timeout = timeout }
}

func WithServerTCPOptions(opts ...netTcp.Option) ServerOption {
	return func(s *Server) { s.tcpOptions = append(s.tcpOptions, opts...) }
}

type serverConn struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type Server struct {
	handlers    map[string]Handler
	timeout     time.Duration
	tcpOptions  []netTcp.Option
	slots       chan struct{}
	mu          sync.Mutex
	listener    net.Listener
	connections map[transport.Conn]serverConn
	stopping    bool
	wait        sync.WaitGroup
	stopOnce    sync.Once
	done        chan struct{}
}

func NewServer(opts ...ServerOption) *Server {
	s := &Server{handlers: make(map[string]Handler), timeout: 30 * time.Second,
		slots: make(chan struct{}, 1024), connections: make(map[transport.Conn]serverConn), done: make(chan struct{})}
	for _, option := range opts {
		option(s)
	}
	if s.timeout <= 0 || cap(s.slots) == 0 {
		panic("rpc: timeout and max concurrent must be positive")
	}
	return s
}

// Register 在 Serve 前注册，重复注册直接覆盖。
func (s *Server) Register(method string, handler Handler) { s.handlers[method] = handler }

func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		_ = listener.Close()
		return ErrClosed
	}
	if s.listener != nil {
		s.mu.Unlock()
		panic("rpc: server already serving")
	}
	s.listener = listener
	s.wait.Add(1)
	s.mu.Unlock()
	defer s.wait.Done()
	tcp := netTcp.NewListener(listener, s.tcpOptions...)
	for {
		conn, err := tcp.Accept()
		if err != nil {
			s.mu.Lock()
			stopping := s.stopping
			s.mu.Unlock()
			if stopping {
				return nil
			}
			return err
		}
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			_ = conn.Close()
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.connections[conn] = serverConn{ctx: ctx, cancel: cancel}
		s.mu.Unlock()
		if err := conn.Start(s); err != nil {
			_ = conn.Close()
		}
	}
}

func (s *Server) HandleMessage(conn transport.Conn, message *packet.Message) {
	request := new(Request)
	if message.Type != packet.Req || message.Route != 1 || proto.Unmarshal(message.Body, request) != nil {
		_ = conn.Close()
		return
	}
	s.mu.Lock()
	connection, ok := s.connections[conn]
	if s.stopping || !ok {
		s.mu.Unlock()
		return
	}
	select {
	case s.slots <- struct{}{}:
		s.wait.Add(1)
		s.mu.Unlock()
		go s.process(connection.ctx, conn, message.Seq, request)
	default:
		s.mu.Unlock()
		s.reply(connection.ctx, conn, message.Seq, &Response{Failure: ErrBusy})
	}
}

func (s *Server) process(connectionCtx context.Context, conn transport.Conn, seq uint64, request *Request) {
	defer s.wait.Done()
	defer func() { <-s.slots }()
	timeout := s.timeout
	if request.TimeoutNanos != 0 {
		timeout = min(timeout, time.Duration(request.TimeoutNanos))
	}
	ctx, cancel := context.WithTimeout(connectionCtx, timeout)
	defer cancel()
	result := &Response{}
	handler := s.handlers[request.Method]
	if handler == nil {
		result.Failure = NewError(CodeNotFound, "rpc: method not found")
	} else if err := ctx.Err(); err != nil {
		result.Failure = FromError(err)
	} else {
		response, failure := handler(ctx, request.Body)
		result.Failure = failure
		if failure == nil && seq != 0 {
			var err error
			result.Body, err = proto.Marshal(response)
			if err != nil {
				result.Failure = FromError(err)
			}
		}
	}
	s.reply(connectionCtx, conn, seq, result)
}

func (s *Server) reply(ctx context.Context, conn transport.Conn, seq uint64, result *Response) {
	if seq == 0 {
		if result.Failure != nil {
			logx.Errorf("rpc: send handler failed: %v", result.Failure)
		}
		return
	}
	body, err := proto.Marshal(result)
	if err == nil {
		err = conn.WriteContext(ctx, &packet.Message{Type: packet.Rsp, Route: 1, Seq: seq, Body: body})
	}
	if err != nil {
		_ = conn.Close()
	}
}

func (s *Server) HandleClose(conn transport.Conn) {
	s.mu.Lock()
	connection, ok := s.connections[conn]
	delete(s.connections, conn)
	s.mu.Unlock()
	if ok {
		connection.cancel()
	}
}

// Shutdown 停止接收新调用，等在途调用回复；超时则关闭连接并取消处理上下文。
func (s *Server) Shutdown(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopping = true
		listener := s.listener
		s.mu.Unlock()
		if listener != nil {
			_ = listener.Close()
		}
		go func() {
			s.wait.Wait()
			s.closeConnections()
			close(s.done)
		}()
	})
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.closeConnections()
		return ctx.Err()
	}
}

func (s *Server) closeConnections() {
	s.mu.Lock()
	connections := make([]transport.Conn, 0, len(s.connections))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}
