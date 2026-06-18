package ws

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network/client"
	"github.com/2comjie/wali/network/server"
	"github.com/gorilla/websocket"
)

// NewListener 创建 WebSocket server.Listener
func NewListener(opts ...Option) server.Listener {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &listener{
		options:  options,
		upgrader: &websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

// Dial 创建 WebSocket client.Transport（客户端连接）
func Dial(address string, opts ...Option) (client.Transport, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	scheme := "ws"
	if options.certFile != "" {
		scheme = "wss"
	}
	url := fmt.Sprintf("%s://%s%s", scheme, address, options.path)

	dialer := websocket.DefaultDialer
	if options.certFile != "" {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return newTransport(conn), nil
}

type listener struct {
	options   *options
	upgrader  *websocket.Upgrader
	httpSrv   *http.Server
	addr      net.Addr
	connCh    chan *transport
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (l *listener) Close() error {
	var err error
	l.closeOnce.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
		if l.httpSrv != nil {
			err = l.httpSrv.Close()
		}
	})
	return err
}

func (l *listener) Listen(address string) error {
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.connCh = make(chan *transport, 128)

	mux := http.NewServeMux()
	mux.HandleFunc(l.options.path, l.handleWS)

	l.httpSrv = &http.Server{
		Addr:    address,
		Handler: mux,
	}
	l.httpSrv.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	if l.options.certFile != "" && l.options.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(l.options.certFile, l.options.keyFile)
		if err != nil {
			_ = ln.Close()
			return err
		}
		ln = tls.NewListener(ln, &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{},
		})
	}

	l.addr = ln.Addr()

	go func() {
		if err := l.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logx.Errorf("ws serve error: %v", err)
		}
	}()

	return nil
}

func (l *listener) Addr() net.Addr {
	return l.addr
}

func (l *listener) Accept() (server.Transport, error) {
	select {
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case trans, ok := <-l.connCh:
		if !ok {
			return nil, net.ErrClosed
		}
		return trans, nil
	}
}

func (l *listener) Protocol() string {
	return "ws"
}

func (l *listener) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := l.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Warnf("ws accept error: %v", err)
		return
	}

	trans := newTransport(conn)

	select {
	case <-l.ctx.Done():
		_ = conn.Close()
		return
	case l.connCh <- trans:
	}
}
