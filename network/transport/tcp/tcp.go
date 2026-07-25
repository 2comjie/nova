package tcp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/2comjie/wali/network/transport"
	"github.com/2comjie/wali/network/transport/internal/netconn"
	"github.com/2comjie/wali/packet"
)

type options struct {
	tlsConfig   *tls.Config
	codec       *packet.Codec
	writeQueue  int
	writeWait   time.Duration
	keepAlive   time.Duration
	dialTimeout time.Duration
}

// Option 配置 TCP Listener 或 Dialer。
type Option func(*options)

// WithTLS 启用 TLS，并强制最低 TLS 1.3。
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

// WithCodec 设置包编解码器。
func WithCodec(codec *packet.Codec) Option {
	return func(options *options) {
		options.codec = codec
	}
}

// WithWriteQueue 设置每个连接的写队列长度。
func WithWriteQueue(size int) Option {
	return func(options *options) {
		options.writeQueue = size
	}
}

// WithWriteTimeout 设置单次底层写入超时。
func WithWriteTimeout(timeout time.Duration) Option {
	return func(options *options) {
		options.writeWait = timeout
	}
}

// WithKeepAlive 设置 TCP keepalive 周期。
func WithKeepAlive(period time.Duration) Option {
	return func(options *options) {
		options.keepAlive = period
	}
}

// WithDialTimeout 设置 TCP 建连超时。
func WithDialTimeout(timeout time.Duration) Option {
	return func(options *options) {
		options.dialTimeout = timeout
	}
}

type listener struct {
	net.Listener
	options options
}

// Listen 监听 TCP 地址。
func Listen(address string, opts ...Option) (transport.Listener, error) {
	raw, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return NewListener(raw, opts...), nil
}

// NewListener 将已有 net.Listener 接入 network。
func NewListener(raw net.Listener, opts ...Option) transport.Listener {
	options := options{}
	for _, option := range opts {
		option(&options)
	}
	return &listener{
		Listener: raw,
		options:  options,
	}
}

func (l *listener) Accept() (transport.Conn, error) {
	raw, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if conn, ok := raw.(*net.TCPConn); ok && l.options.keepAlive > 0 {
		_ = conn.SetKeepAlive(true)
		_ = conn.SetKeepAlivePeriod(l.options.keepAlive)
	}

	secure := l.options.tlsConfig != nil
	if secure {
		raw = tls.Server(raw, l.options.tlsConfig)
	}
	return netconn.New(
		raw,
		l.options.codec,
		transport.TypeTCP,
		secure,
		l.options.writeQueue,
		l.options.writeWait,
	), nil
}

type dialer struct {
	address string
	options options
}

// NewDialer 创建 TCP 客户端拨号器。
func NewDialer(address string, opts ...Option) transport.Dialer {
	options := options{}
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
		return nil, errors.New("network/tcp: 禁止InsecureSkipVerify")
	}

	netDialer := &net.Dialer{
		Timeout:   d.options.dialTimeout,
		KeepAlive: d.options.keepAlive,
	}
	raw, err := netDialer.DialContext(ctx, "tcp", d.address)
	if err != nil {
		return nil, err
	}

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
		transport.TypeTCP,
		secure,
		d.options.writeQueue,
		d.options.writeWait,
	), nil
}
