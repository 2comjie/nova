package rpc

import (
	"context"
	"sync"
	"time"

	"github.com/2comjie/nova/network/transport"
	netTcp "github.com/2comjie/nova/network/transport/tcp"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

// Invoker 是生成的服务客户端需要的调用能力。
type Invoker interface {
	Invoke(context.Context, string, proto.Message, proto.Message) error
}

type ConnOption func(*Conn)

func WithCallTimeout(timeout time.Duration) ConnOption {
	return func(c *Conn) { c.timeout = timeout }
}

func WithMaxPending(limit int) ConnOption {
	return func(c *Conn) { c.maxPending = limit }
}

func WithTCPOptions(opts ...netTcp.Option) ConnOption {
	return func(c *Conn) { c.tcpOptions = append(c.tcpOptions, opts...) }
}

// Conn 按需建立一条 TCP 连接；断线只结束在途调用，下次调用重新连接。
type Conn struct {
	address    string
	timeout    time.Duration
	maxPending int
	tcpOptions []netTcp.Option
	dial       chan struct{}
	mu         sync.Mutex
	conn       transport.Conn
	seq        uint64
	pending    map[uint64]chan *Response
	closed     bool
}

func NewConn(address string, opts ...ConnOption) *Conn {
	c := &Conn{address: address, timeout: 10 * time.Second, maxPending: 1024,
		pending: make(map[uint64]chan *Response), dial: make(chan struct{}, 1)}
	for _, option := range opts {
		option(c)
	}
	if c.timeout <= 0 || c.maxPending <= 0 {
		panic("rpc: timeout and max pending must be positive")
	}
	return c
}

func (c *Conn) Target() string { return c.address }

func (c *Conn) connection(ctx context.Context) (transport.Conn, error) {
	select {
	case c.dial <- struct{}{}:
		defer func() { <-c.dial }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		return conn, nil
	}
	conn, err := netTcp.NewDialer(c.address, c.tcpOptions...).DialContext(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return nil, ErrClosed
	}
	c.conn = conn
	c.mu.Unlock()
	if err := conn.Start(c); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *Conn) Invoke(ctx context.Context, method string, request, response proto.Message) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	conn, err := c.connection(ctx)
	if err != nil {
		return err
	}
	body, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	body, err = proto.Marshal(&Request{Method: method, Body: body, TimeoutNanos: int64(time.Until(deadline))})
	if err != nil {
		return err
	}
	call := make(chan *Response, 1)
	c.mu.Lock()
	if c.closed || c.conn != conn {
		c.mu.Unlock()
		return ErrClosed
	}
	if len(c.pending) >= c.maxPending {
		c.mu.Unlock()
		return ErrBusy
	}
	c.seq++
	if c.seq == 0 {
		c.seq++
	}
	seq := c.seq
	c.pending[seq] = call
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
	}()
	if err := conn.WriteContext(ctx, &packet.Message{Type: packet.Req, Route: 1, Seq: seq, Body: body}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result, ok := <-call:
		if !ok {
			return ErrClosed
		}
		if result.Failure != nil {
			return result.Failure
		}
		return proto.Unmarshal(result.Body, response)
	}
}

// Send 只确认请求已写出，不等待远端业务执行，也不自动重试。
func (c *Conn) Send(ctx context.Context, method string, request proto.Message) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	conn, err := c.connection(ctx)
	if err != nil {
		return err
	}
	body, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	body, err = proto.Marshal(&Request{Method: method, Body: body, TimeoutNanos: int64(time.Until(deadline))})
	if err != nil {
		return err
	}
	return conn.WriteContext(ctx, &packet.Message{Type: packet.Req, Route: 1, Body: body})
}

func (c *Conn) HandleMessage(conn transport.Conn, message *packet.Message) {
	result := new(Response)
	if message.Type != packet.Rsp || message.Route != 1 || message.Seq == 0 || proto.Unmarshal(message.Body, result) != nil {
		_ = conn.Close()
		return
	}
	c.mu.Lock()
	call := c.pending[message.Seq]
	delete(c.pending, message.Seq)
	c.mu.Unlock()
	if call != nil {
		call <- result
	}
}

func (c *Conn) HandleClose(conn transport.Conn) {
	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
		for seq, call := range c.pending {
			delete(c.pending, seq)
			close(call)
		}
	}
	c.mu.Unlock()
}

func (c *Conn) Close() error {
	c.mu.Lock()
	c.closed = true
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}
