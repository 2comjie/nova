package ws

import (
	"crypto/tls"
	"net"
	"net/http"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/network"
	"go.uber.org/zap"
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

func (s *server) Start(opts ...network.Option) error {
	o := network.DefaultOption()
	for _, opt := range opts {
		opt(o)
	}
	s.baseOptions = o
	s.connMgr = network.NewConnMgr(o)

	ready := make(chan struct{})
	help.SafeGo(func() { s.serve(ready) })
	<-ready

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

func (s *server) serve(ready chan struct{}) {
	mux := http.NewServeMux()
	mux.HandleFunc(s.options.path, s.handleWS)

	s.httpServer = &http.Server{
		Addr:    s.options.addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", s.options.addr)
	if err != nil {
		zap.S().Errorf("ws listen error: %v", err)
		close(ready)
		return
	}

	close(ready)

	if s.options.certFile != "" && s.options.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.options.certFile, s.options.keyFile)
		if err != nil {
			zap.S().Errorf("ws tls error: %v", err)
			return
		}
		tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		_ = s.httpServer.Serve(tlsLn)
		return
	}
	_ = s.httpServer.Serve(ln)
}

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 跨域由业务层控制
	})
	if err != nil {
		zap.S().Warnf("ws accept error: %v", err)
		return
	}
	trans := newTransport(r.Context(), conn)
	if err = s.connMgr.Add(trans); err != nil {
		zap.S().Errorf("connection allocate error: %v", err)
		_ = conn.Close(websocket.StatusPolicyViolation, "too many connections")
	}
}
