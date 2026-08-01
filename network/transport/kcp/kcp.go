package netKcp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/network/transport"
	"github.com/2comjie/wali/network/transport/internal/netconn"
	"github.com/2comjie/wali/packet"
	kcpgo "github.com/xtaci/kcp-go/v5"
)

type options struct {
	block       kcpgo.BlockCrypt
	dataShards  int
	parityShard int
	tlsConfig   *tls.Config
	codec       *packet.Codec
	writeQueue  int
	writeWait   time.Duration
	noDelay     int
	interval    int
	resend      int
	noCongest   int
	sendWindow  int
	readWindow  int
	mtu         int
}

type Option func(*options)

func defaultOptions() options {
	return options{
		dataShards:  10,
		parityShard: 3,
		noDelay:     1,
		interval:    10,
		resend:      2,
		noCongest:   1,
		sendWindow:  128,
		readWindow:  128,
		mtu:         1400,
	}
}

func WithBlockCrypt(block kcpgo.BlockCrypt) Option {
	return func(options *options) {
		options.block = block
	}
}

func WithFEC(dataShards, parityShards int) Option {
	return func(options *options) {
		options.dataShards = dataShards
		options.parityShard = parityShards
	}
}

func WithTLS(config *tls.Config) Option {
	return func(options *options) {
		if config == nil {
			return
		}
		options.tlsConfig = config.Clone()
		if options.tlsConfig.MinVersion < tls.VersionTLS13 {
			options.tlsConfig.MinVersion = tls.VersionTLS13
		}
	}
}

func WithCodec(codec *packet.Codec) Option {
	return func(options *options) {
		options.codec = codec
	}
}

func WithWriteQueue(size int) Option {
	return func(options *options) {
		options.writeQueue = size
	}
}

func WithWriteTimeout(timeout time.Duration) Option {
	return func(options *options) {
		options.writeWait = timeout
	}
}

func WithNoDelay(noDelay, interval, resend, noCongest int) Option {
	return func(options *options) {
		options.noDelay = noDelay
		options.interval = interval
		options.resend = resend
		options.noCongest = noCongest
	}
}

func WithWindowSize(sendWindow, readWindow int) Option {
	return func(options *options) {
		options.sendWindow = sendWindow
		options.readWindow = readWindow
	}
}

func WithMTU(mtu int) Option {
	return func(options *options) {
		options.mtu = mtu
	}
}

type listener struct {
	raw     *kcpgo.Listener
	options options
}

func Listen(address string, opts ...Option) (transport.Listener, error) {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	raw, err := kcpgo.ListenWithOptions(address, options.block, options.dataShards, options.parityShard)
	if err != nil {
		return nil, err
	}
	return &listener{
		raw:     raw,
		options: options,
	}, nil
}

func (l *listener) Accept() (transport.Conn, error) {
	session, err := l.raw.AcceptKCP()
	if err != nil {
		return nil, err
	}
	configureSession(session, l.options)

	var raw net.Conn = session
	secure := l.options.tlsConfig != nil
	if secure {
		raw = tls.Server(raw, l.options.tlsConfig)
	}
	return netconn.New(
		raw,
		l.options.codec,
		transport.TypeKCP,
		secure,
		l.options.writeQueue,
		l.options.writeWait,
	), nil
}

func (l *listener) Close() error {
	return l.raw.Close()
}

func (l *listener) Addr() net.Addr {
	return l.raw.Addr()
}

type dialer struct {
	address string
	options options
}

func NewDialer(address string, opts ...Option) transport.Dialer {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	return &dialer{
		address: address,
		options: options,
	}
}

func (d *dialer) DialContext(ctx context.Context) (transport.Conn, error) {
	if d.options.tlsConfig != nil && d.options.tlsConfig.InsecureSkipVerify {
		return nil, errors.New("network/kcp: 禁止InsecureSkipVerify")
	}

	result := make(chan struct {
		session *kcpgo.UDPSession
		err     error
	}, 1)
	help.SafeGo(func() {
		session, err := kcpgo.DialWithOptions(
			d.address,
			d.options.block,
			d.options.dataShards,
			d.options.parityShard,
		)
		result <- struct {
			session *kcpgo.UDPSession
			err     error
		}{session: session, err: err}
	})

	var session *kcpgo.UDPSession
	select {
	case <-ctx.Done():
		help.SafeGo(func() {
			value := <-result
			if value.session != nil {
				_ = value.session.Close()
			}
		})
		return nil, ctx.Err()
	case value := <-result:
		if value.err != nil {
			return nil, value.err
		}
		session = value.session
	}

	configureSession(session, d.options)
	var raw net.Conn = session
	secure := d.options.tlsConfig != nil
	if secure {
		tlsConn := tls.Client(raw, d.options.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		raw = tlsConn
	}

	return netconn.New(
		raw,
		d.options.codec,
		transport.TypeKCP,
		secure,
		d.options.writeQueue,
		d.options.writeWait,
	), nil
}

func configureSession(session *kcpgo.UDPSession, options options) {
	session.SetStreamMode(true)
	session.SetNoDelay(options.noDelay, options.interval, options.resend, options.noCongest)
	session.SetWindowSize(options.sendWindow, options.readWindow)
	if options.mtu > 0 {
		session.SetMtu(options.mtu)
	}
}
