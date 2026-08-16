package netWs

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
	"github.com/gorilla/websocket"
)

const (
	defaultPath       = "/"
	defaultWriteQueue = 256
	defaultWriteWait  = 10 * time.Second
	maxQueuedBytes    = 4 << 20
)

type options struct {
	path        string
	tlsConfig   *tls.Config
	codec       *packet.Codec
	writeQueue  int
	writeWait   time.Duration
	originCheck func(*http.Request) bool
	header      http.Header
}

type Option func(*options)

func defaultOptions() options {
	return options{
		path:       defaultPath,
		writeQueue: defaultWriteQueue,
		writeWait:  defaultWriteWait,
	}
}

func WithPath(path string) Option {
	return func(options *options) {
		if path != "" {
			options.path = path
		}
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
		if size > 0 {
			options.writeQueue = size
		}
	}
}

func WithWriteTimeout(timeout time.Duration) Option {
	return func(options *options) {
		if timeout > 0 {
			options.writeWait = timeout
		}
	}
}

func WithOriginCheck(check func(*http.Request) bool) Option {
	return func(options *options) {
		options.originCheck = check
	}
}

func WithHeader(header http.Header) Option {
	return func(options *options) {
		options.header = header.Clone()
	}
}

type listener struct {
	raw       net.Listener
	options   options
	server    *http.Server
	accepts   chan transport.Conn
	errs      chan error
	done      chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
	serverMu  sync.Mutex
}

func Listen(address string, opts ...Option) (transport.Listener, error) {
	raw, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return NewListener(raw, opts...), nil
}

func NewListener(raw net.Listener, opts ...Option) transport.Listener {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	return &listener{
		raw:     raw,
		options: options,
		accepts: make(chan transport.Conn, 128),
		errs:    make(chan error, 1),
		done:    make(chan struct{}),
	}
}

func (l *listener) Accept() (transport.Conn, error) {
	l.startOnce.Do(func() {
		upgrader := websocket.Upgrader{
			EnableCompression: false,
			Subprotocols:      []string{"nova"},
			CheckOrigin: func(request *http.Request) bool {
				if l.options.originCheck != nil {
					return l.options.originCheck(request)
				}
				origin := request.Header.Get("Origin")
				if origin == "" {
					return true
				}
				parsed, err := url.Parse(origin)
				return err == nil && strings.EqualFold(parsed.Host, request.Host)
			},
		}

		mux := http.NewServeMux()
		mux.HandleFunc(l.options.path, func(writer http.ResponseWriter, request *http.Request) {
			conn, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			conn.SetReadLimit(packet.DefaultMaxFrame)
			value := newConn(
				conn,
				l.options.codec,
				l.options.tlsConfig != nil,
				l.options.writeQueue,
				l.options.writeWait,
			)
			select {
			case l.accepts <- value:
			case <-l.done:
				_ = conn.Close()
			}
		})

		server := &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		l.serverMu.Lock()
		l.server = server
		l.serverMu.Unlock()
		help.SafeGo(func() {
			raw := l.raw
			if l.options.tlsConfig != nil {
				raw = tls.NewListener(raw, l.options.tlsConfig)
			}
			err := server.Serve(raw)
			if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
				select {
				case l.errs <- err:
				default:
				}
			}
		})
	})

	select {
	case conn := <-l.accepts:
		return conn, nil
	case err := <-l.errs:
		return nil, err
	case <-l.done:
		return nil, transport.ErrClosed
	}
}

func (l *listener) Close() error {
	var closeErr error
	l.closeOnce.Do(func() {
		close(l.done)
		l.serverMu.Lock()
		server := l.server
		l.serverMu.Unlock()
		if server != nil {
			closeErr = server.Close()
		} else {
			closeErr = l.raw.Close()
		}
	})
	if errors.Is(closeErr, http.ErrServerClosed) || errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
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
	parsed, err := url.Parse(address)
	if err == nil && options.path != defaultPath && (parsed.Path == "" || parsed.Path == defaultPath) {
		parsed.Path = options.path
		address = parsed.String()
	}
	return &dialer{
		address: address,
		options: options,
	}
}

func (d *dialer) DialContext(ctx context.Context) (transport.Conn, error) {
	if d.options.tlsConfig != nil && d.options.tlsConfig.InsecureSkipVerify {
		return nil, errors.New("network/ws: 禁止InsecureSkipVerify")
	}

	dialer := websocket.Dialer{
		TLSClientConfig:   d.options.tlsConfig,
		Subprotocols:      []string{"nova"},
		EnableCompression: false,
	}
	conn, response, err := dialer.DialContext(ctx, d.address, d.options.header)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, err
	}
	conn.SetReadLimit(packet.DefaultMaxFrame)
	return newConn(
		conn,
		d.options.codec,
		d.options.tlsConfig != nil || strings.HasPrefix(d.address, "wss://"),
		d.options.writeQueue,
		d.options.writeWait,
	), nil
}

type writeRequest struct {
	message *packet.Message
	result  chan error
	size    int64
}

type conn struct {
	raw          *websocket.Conn
	codec        *packet.Codec
	secure       bool
	writeWait    time.Duration
	writes       chan writeRequest
	done         chan struct{}
	startOnce    sync.Once
	closeOnce    sync.Once
	notifyOnce   sync.Once
	handlerMutex sync.RWMutex
	handler      transport.Handler
	queuedBytes  atomic.Int64
}

func newConn(raw *websocket.Conn, codec *packet.Codec, secure bool, writeQueue int, writeWait time.Duration) *conn {
	if codec == nil {
		codec = packet.NewCodec(packet.DefaultMaxFrame)
	}
	return &conn{
		raw:       raw,
		codec:     codec,
		secure:    secure,
		writeWait: writeWait,
		writes:    make(chan writeRequest, writeQueue),
		done:      make(chan struct{}),
	}
}

func (c *conn) Start(handler transport.Handler) error {
	if handler == nil {
		return errors.New("network: 连接Handler不能为空")
	}

	started := false
	var startErr error
	c.startOnce.Do(func() {
		started = true
		c.handlerMutex.Lock()
		c.handler = handler
		c.handlerMutex.Unlock()
		select {
		case <-c.done:
			startErr = transport.ErrClosed
			c.notifyOnce.Do(func() {
				help.SafeRun(func() {
					handler.HandleClose(c)
				})
			})
			return
		default:
		}
		help.SafeGo(c.readLoop)
		help.SafeGo(c.writeLoop)
	})
	if !started {
		return transport.ErrStarted
	}
	return startErr
}

func (c *conn) Write(message *packet.Message) error {
	if message == nil {
		return packet.ErrType
	}
	request := writeRequest{
		message: message,
		result:  make(chan error, 1),
		size:    int64(packet.HeaderSize + len(message.Body)),
	}
	if c.queuedBytes.Add(request.size) > maxQueuedBytes {
		c.queuedBytes.Add(-request.size)
		_ = c.Close()
		return transport.ErrWriteQueueFull
	}
	select {
	case <-c.done:
		c.queuedBytes.Add(-request.size)
		return transport.ErrClosed
	case c.writes <- request:
	default:
		c.queuedBytes.Add(-request.size)
		_ = c.Close()
		return transport.ErrWriteQueueFull
	}

	select {
	case err := <-request.result:
		return err
	case <-c.done:
		return transport.ErrClosed
	}
}

func (c *conn) readLoop() {
	defer c.Close()
	for {
		messageType, reader, err := c.raw.NextReader()
		if err != nil {
			return
		}
		if messageType != websocket.BinaryMessage {
			return
		}

		message, err := c.codec.Read(reader)
		if err != nil {
			return
		}
		var trailing [1]byte
		if count, readErr := reader.Read(trailing[:]); count != 0 || !errors.Is(readErr, io.EOF) {
			message.Release()
			return
		}

		c.handlerMutex.RLock()
		handler := c.handler
		c.handlerMutex.RUnlock()
		if handler != nil {
			help.SafeRun(func() {
				handler.HandleMessage(c, message)
			})
		}
		message.Release()

		select {
		case <-c.done:
			return
		default:
		}
	}
}

func (c *conn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case request := <-c.writes:
			c.queuedBytes.Add(-request.size)
			frame, err := c.codec.Encode(request.message)
			if err == nil {
				_ = c.raw.SetWriteDeadline(time.Now().Add(c.writeWait))
				err = c.raw.WriteMessage(websocket.BinaryMessage, frame.Bytes())
				frame.Release()
			}
			request.result <- err
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func (c *conn) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		close(c.done)
		closeErr = c.raw.Close()

		c.handlerMutex.RLock()
		handler := c.handler
		c.handlerMutex.RUnlock()
		if handler != nil {
			c.notifyOnce.Do(func() {
				help.SafeRun(func() {
					handler.HandleClose(c)
				})
			})
		}
	})
	if errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func (c *conn) LocalAddr() net.Addr {
	return c.raw.LocalAddr()
}

func (c *conn) RemoteAddr() net.Addr {
	return c.raw.RemoteAddr()
}

func (c *conn) Type() transport.Type {
	return transport.TypeWS
}

func (c *conn) Secure() bool {
	return c.secure
}
