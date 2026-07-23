package tcp

import (
	"crypto/tls"
	"net"

	"github.com/2comjie/wali/network/client"
	"github.com/2comjie/wali/network/server"
)

func NewListener(opts ...Option) netServer.Listener {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	ln := &listener{
		ln:      nil,
		options: options,
	}
	return ln
}

func Dial(address string, opts ...Option) (client.Transport, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	netAddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}

	if options.certFile != "" && options.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(options.certFile, options.keyFile)
		if err != nil {
			return nil, err
		}
		conn, err := tls.Dial("tcp", netAddr.String(), &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, err
		}
		return &transport{rawConn: conn}, nil
	}

	conn, err := net.DialTCP("tcp", nil, netAddr)
	if err != nil {
		return nil, err
	}
	return &transport{rawConn: conn}, nil
}

type listener struct {
	ln      *net.TCPListener
	options *options
}

func (l *listener) Close() error {
	return l.ln.Close()
}

func (l *listener) Listen(address string) error {
	netAddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return err
	}

	if l.options.certFile != "" && l.options.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(l.options.certFile, l.options.keyFile)
		if err != nil {
			return err
		}
		ln, err := tls.Listen(netAddr.Network(), netAddr.String(), &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if err != nil {
			return err
		}
		l.ln = ln.(*net.TCPListener)
	} else {
		ln, err := net.ListenTCP("tcp", netAddr)
		if err != nil {
			return err
		}
		l.ln = ln
	}
	return nil
}

func (l *listener) Addr() net.Addr {
	return l.ln.Addr()
}

func (l *listener) Accept() (netServer.Transport, error) {
	conn, err := l.ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	return &transport{rawConn: conn}, nil
}

func (l *listener) Protocol() string {
	return "tcp"
}
