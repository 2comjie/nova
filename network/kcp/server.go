package kcp

import (
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/network"
	kcp "github.com/xtaci/kcp-go"
	"go.uber.org/zap"
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
			zap.S().Warnf("kcp accept error: %v", err)
			return
		}
		if err = s.connMgr.Add(&kcpTransport{sess: sess}); err != nil {
			zap.S().Errorf("connection allocate error: %v", err)
			_ = sess.Close()
		}
	}
}
