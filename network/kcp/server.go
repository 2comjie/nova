package kcp

import (
	"errors"
	"net"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network"
	kcp "github.com/xtaci/kcp-go"
)

const protocol = "kcp"

type server struct {
	options     *options
	baseOptions *network.Options

	listener *kcp.Listener
	connMgr  *network.BaseConnMgr
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

	ln, err := kcp.ListenWithOptions(s.options.addr, nil, 0, 0)
	if err != nil {
		return err
	}
	s.listener = ln

	ready := make(chan struct{})
	help.SafeGo(func() { s.serve(ready) })
	<-ready

	if o.OnStart != nil {
		o.OnStart()
	}
	return nil
}

func (s *server) Stop() error {
	if s.listener == nil {
		return nil
	}
	if s.baseOptions != nil && s.baseOptions.BeforeStop != nil {
		s.baseOptions.BeforeStop()
	}
	err := s.listener.Close()
	s.connMgr.Close("server stop")
	if s.baseOptions != nil && s.baseOptions.OnStop != nil {
		s.baseOptions.OnStop()
	}
	return err
}

func (s *server) serve(ready chan struct{}) {
	close(ready)
	for {
		sess, err := s.listener.AcceptKCP()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logx.Warnf("kcp accept error: %v", err)
			}
			return
		}
		if err = s.connMgr.Add(&kcpTransport{sess: sess}); err != nil {
			logx.Errorf("connection allocate error: %v", err)
			_ = sess.Close()
		}
	}
}
