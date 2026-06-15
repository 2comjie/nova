package ws

import (
	"crypto/tls"
	"net"
	"net/http"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network"
	"nhooyr.io/websocket"
)

const protocol = "ws"

type server struct {
	options     *options
	baseOptions *network.Options

	httpServer *http.Server
	connMgr    *network.BaseConnMgr
}

func New(opts ...Option) network.Server {
	o := defaultOption()
	for _, opt := range opts {
		opt(o)
	}
	return &server{options: o}
}

func (s *server) Addr() string     { return s.options.addr }
func (s *server) Protocol() string { return protocol }

func (s *server) Conn(id int64) (network.Conn, bool) {
	if s.connMgr == nil {
		return nil, false
	}
	return s.connMgr.Conn(id)
}

func (s *server) ConnByUID(uid string) (network.Conn, bool) {
	if s.connMgr == nil {
		return nil, false
	}
	return s.connMgr.ConnByUID(uid)
}

func (s *server) BindUID(connID int64, uid string) error {
	if s.connMgr == nil {
		return network.ErrConnNotFound
	}
	return s.connMgr.BindUID(connID, uid)
}

func (s *server) UnbindUID(connID int64) (string, error) {
	if s.connMgr == nil {
		return "", network.ErrConnNotFound
	}
	return s.connMgr.UnbindUID(connID)
}

func (s *server) VisitConns(fn func(network.Conn) bool) {
	if s.connMgr == nil {
		return
	}
	s.connMgr.Visit(fn)
}

func (s *server) Stat() int64 {
	if s.connMgr == nil {
		return 0
	}
	return s.connMgr.Stat()
}

func (s *server) Start(opts ...network.Option) error {
	o := network.DefaultOption()
	for _, opt := range opts {
		opt(o)
	}
	s.baseOptions = o
	s.connMgr = network.NewConnMgr(o)

	ready := make(chan error, 1)
	help.SafeGo(func() { s.serve(ready) })
	if err := <-ready; err != nil {
		return err
	}

	if o.OnStart != nil {
		o.OnStart()
	}
	return nil
}

func (s *server) Stop() error {
	if s.httpServer == nil {
		return nil
	}
	if s.baseOptions != nil && s.baseOptions.BeforeStop != nil {
		s.baseOptions.BeforeStop()
	}
	s.connMgr.Close("server stop")
	err := s.httpServer.Close()
	if s.baseOptions != nil && s.baseOptions.OnStop != nil {
		s.baseOptions.OnStop()
	}
	return err
}

func (s *server) serve(ready chan error) {
	mux := http.NewServeMux()
	mux.HandleFunc(s.options.path, s.handleWS)

	s.httpServer = &http.Server{
		Addr:    s.options.addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", s.options.addr)
	if err != nil {
		logx.Errorf("ws listen error: %v", err)
		ready <- err
		return
	}

	if s.options.certFile != "" && s.options.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.options.certFile, s.options.keyFile)
		if err != nil {
			logx.Errorf("ws tls error: %v", err)
			_ = ln.Close()
			ready <- err
			return
		}
		ready <- nil
		tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		_ = s.httpServer.Serve(tlsLn)
		return
	}
	ready <- nil
	_ = s.httpServer.Serve(ln)
}

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 跨域由业务层控制
	})
	if err != nil {
		logx.Warnf("ws accept error: %v", err)
		return
	}
	trans := newTransport(r.Context(), conn)
	if err = s.connMgr.Add(trans); err != nil {
		logx.Errorf("connection allocate error: %v", err)
		_ = conn.Close(websocket.StatusPolicyViolation, "too many connections")
	}
}
