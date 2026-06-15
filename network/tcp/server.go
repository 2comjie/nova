package tcp

import (
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network"
)

const protocol = "tcp"

type server struct {
	options     *options
	baseOptions *network.Options

	listener net.Listener
	connMgr  *network.BaseConnMgr
}

func New(opts ...Option) network.Server {
	option := defaultOption()
	for _, opt := range opts {
		opt(option)
	}
	return &server{options: option}
}

func (s *server) Addr() string {
	return s.options.addr
}

func (s *server) Start(opts ...network.Option) error {
	if err := s.init(); err != nil {
		return err
	}

	option := network.DefaultOption()
	for _, opt := range opts {
		opt(option)
	}
	s.baseOptions = option
	s.connMgr = network.NewConnMgr(option)

	ready := make(chan struct{})
	help.SafeGo(func() { s.serve(ready) })
	<-ready

	if option.OnStart != nil {
		option.OnStart()
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
	if err := s.listener.Close(); err != nil {
		return err
	}
	s.connMgr.Close("server stop")
	if s.baseOptions != nil && s.baseOptions.OnStop != nil {
		s.baseOptions.OnStop()
	}
	return nil
}

func (s *server) Protocol() string {
	return protocol
}

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

func (s *server) init() error {
	netAddr, err := net.ResolveTCPAddr("tcp", s.options.addr)
	if err != nil {
		return err
	}

	if s.options.keyFile != "" && s.options.certFile != "" {
		cert, err := tls.LoadX509KeyPair(s.options.certFile, s.options.keyFile)
		if err != nil {
			return err
		}
		s.listener, err = tls.Listen(netAddr.Network(), netAddr.String(), &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		return err
	}

	s.listener, err = net.ListenTCP(netAddr.Network(), netAddr)
	return err
}

func (s *server) serve(ready chan struct{}) {
	close(ready) // listener 已就绪，通知 Start 继续

	var tempDelay int64 // nanoseconds
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			var e net.Error
			if errors.As(err, &e) && e.Timeout() {
				if tempDelay == 0 {
					tempDelay = 5_000_000 // 5ms
				} else {
					tempDelay *= 2
				}
				if tempDelay > 1_000_000_000 {
					tempDelay = 1_000_000_000
				}
				logx.Warnf("tcp accept error: %v; retrying in %dns", err, tempDelay)
				// net.Error.Timeout() 极少触发，sleep 逻辑保留但已有上界
				time.Sleep(time.Duration(tempDelay))
				continue
			}
			if !errors.Is(err, net.ErrClosed) {
				logx.Warnf("tcp accept error: %v", err)
			}
			return
		}
		tempDelay = 0
		if err = s.connMgr.Add(&tcpTransport{conn: conn}); err != nil {
			logx.Errorf("connection allocate error: %v", err)
			_ = conn.Close()
		}
	}
}
