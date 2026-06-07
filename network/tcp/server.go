package tcp

import (
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/network"
	"go.uber.org/zap"
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

	s := &server{
		options:     option,
		baseOptions: nil,
		listener:    nil,
		connMgr:     nil,
	}
	return s
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
	help.SafeGo(s.serve)
	time.Sleep(10 * time.Millisecond)
	if option.OnStart != nil {
		option.OnStart()
	}
	return nil
}

func (s *server) Stop() error {
	if s.baseOptions.BeforeStop != nil {
		s.baseOptions.BeforeStop()
	}
	if err := s.listener.Close(); err != nil {
		return err
	}
	s.connMgr.Close("server stop")
	if s.baseOptions.OnStop != nil {
		s.baseOptions.OnStop()
	}
	return nil
}

func (s *server) Protocol() string {
	return protocol
}

func (s *server) init() error {
	netAddr, err := net.ResolveTCPAddr("tcp", s.options.addr)
	if err != nil {
		return err
	}

	// 监听
	if s.options.keyFile != "" && s.options.certFile != "" {
		cert, err := tls.LoadX509KeyPair(s.options.certFile, s.options.keyFile)
		if err != nil {
			return err
		}
		s.listener, err = tls.Listen(netAddr.Network(), netAddr.String(), &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if err != nil {
			return err
		}
		return nil
	}

	s.listener, err = net.ListenTCP(netAddr.Network(), netAddr)
	return err
}
func (s *server) serve() {
	var tempDelay time.Duration
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			var e net.Error
			if errors.As(err, &e) && e.Timeout() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if tempDelay > time.Second {
					tempDelay = time.Second
				}
				zap.S().Warnf("tcp accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			zap.S().Warnf("tcp accept error: %v", err)
			return
		}
		tempDelay = 0
		trans := &tcpTransport{conn: conn}
		if err = s.connMgr.Add(trans); err != nil {
			zap.S().Errorf("connection allocate error: %v", err)
			_ = conn.Close()
		}
	}
}
