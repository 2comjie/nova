package kcp

import (
	"net"

	"github.com/2comjie/wali/network"
	kcp "github.com/xtaci/kcp-go"
)

func NewListener(opts ...Option) network.Listener {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &listener{
		options: options,
	}
}

type listener struct {
	ln      *kcp.Listener
	options *options
}

func (l *listener) Close() error {
	if l.ln == nil {
		return nil
	}
	return l.ln.Close()
}

func (l *listener) Listen(address string) error {
	ln, err := kcp.ListenWithOptions(address, l.options.block, l.options.dataShards, l.options.parityShards)
	if err != nil {
		return err
	}
	l.ln = ln
	return nil
}

func (l *listener) Addr() net.Addr {
	if l.ln == nil {
		return nil
	}
	return l.ln.Addr()
}

func (l *listener) Accept() (network.Transport, error) {
	sess, err := l.ln.AcceptKCP()
	if err != nil {
		return nil, err
	}
	return &transport{sess: sess}, nil
}

func (l *listener) Protocol() string {
	return "kcp"
}
