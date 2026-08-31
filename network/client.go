package network

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/network/protocol"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

type PushHandler func(context.Context, []byte)

type callResult struct {
	body []byte
	err  error
}

type pendingCall struct {
	route  uint32
	result chan callResult
}

type bindResult struct {
	response *protocol.BindResponse
	err      error
}

type Client struct {
	options options

	mutex       sync.Mutex
	conn        transport.Conn
	bindWait    chan bindResult
	pending     map[uint64]*pendingCall
	pushHandler map[uint32]PushHandler

	seq          atomic.Uint64
	bound        atomic.Bool
	closed       atomic.Bool
	dialed       atomic.Bool
	pingAt       atomic.Int64
	pingNonce    atomic.Uint64
	heartbeat    time.Duration
	done         chan struct{}
	finishOnce   sync.Once
	heartbeatRun sync.Once
}

func NewClient(opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	if options.dialer == nil {
		return nil, ErrDialerMissing
	}
	return &Client{
		options:     options,
		pending:     make(map[uint64]*pendingCall),
		pushHandler: make(map[uint32]PushHandler),
		heartbeat:   options.heartbeat,
		done:        make(chan struct{}),
	}, nil
}

func (c *Client) Dial(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	if !c.dialed.CompareAndSwap(false, true) {
		return errors.New("network: Client已经拨号")
	}
	conn, err := c.options.dialer.DialContext(ctx)
	if err != nil {
		c.dialed.Store(false)
		return err
	}

	c.mutex.Lock()
	c.conn = conn
	c.mutex.Unlock()
	if err := conn.Start(c); err != nil {
		_ = conn.Close()
		return err
	}
	return nil
}

func (c *Client) Bind(ctx context.Context, token []byte) error {
	if len(token) == 0 || len(token) > c.options.maxToken {
		return ErrUnauthorized
	}
	if c.bound.Load() {
		return ErrAlreadyBound
	}

	c.mutex.Lock()
	if c.conn == nil {
		c.mutex.Unlock()
		return transport.ErrClosed
	}
	if c.bindWait != nil {
		c.mutex.Unlock()
		return errors.New("network: Bind正在进行")
	}
	wait := make(chan bindResult, 1)
	c.bindWait = wait
	conn := c.conn
	c.mutex.Unlock()

	request := &protocol.BindRequest{Token: token}
	body, holder, err := marshalControl(request)
	if err != nil {
		c.clearBindWait(wait)
		return err
	}
	err = conn.Write(&packet.Message{
		Type: packet.BindReq,
		Body: body,
	})
	holder.Release()
	if err != nil {
		c.clearBindWait(wait)
		return err
	}

	defer c.clearBindWait(wait)
	select {
	case result := <-wait:
		if result.err != nil {
			return result.err
		}
		if result.response.Code != protocol.BindCode_BIND_OK {
			return ErrUnauthorized
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClosed
	}
}

func (c *Client) clearBindWait(wait chan bindResult) {
	c.mutex.Lock()
	if c.bindWait == wait {
		c.bindWait = nil
	}
	c.mutex.Unlock()
}

func (c *Client) Call(ctx context.Context, route uint32, body []byte) ([]byte, error) {
	if !c.bound.Load() {
		return nil, ErrNotBound
	}
	seq := c.seq.Add(1)

	body, err := encodeBody(c.options, packet.Req, route, seq, body)
	if err != nil {
		return nil, err
	}
	call := &pendingCall{
		route:  route,
		result: make(chan callResult, 1),
	}

	c.mutex.Lock()
	if len(c.pending) >= c.options.maxPending {
		c.mutex.Unlock()
		return nil, ErrPendingFull
	}
	conn := c.conn
	if conn == nil {
		c.mutex.Unlock()
		return nil, ErrClosed
	}
	c.pending[seq] = call
	c.mutex.Unlock()

	if err := conn.Write(&packet.Message{
		Type:  packet.Req,
		Route: route,
		Seq:   seq,
		Body:  body,
	}); err != nil {
		c.removePending(seq, call)
		return nil, err
	}

	select {
	case result := <-call.result:
		return result.body, result.err
	case <-ctx.Done():
		c.removePending(seq, call)
		return nil, ctx.Err()
	case <-c.done:
		c.removePending(seq, call)
		return nil, ErrClosed
	}
}

func (c *Client) Tell(_ context.Context, route uint32, body []byte) error {
	if !c.bound.Load() {
		return ErrNotBound
	}

	body, err := encodeBody(c.options, packet.Req, route, 0, body)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	conn := c.conn
	c.mutex.Unlock()
	if conn == nil {
		return ErrClosed
	}
	return conn.Write(&packet.Message{
		Type:  packet.Req,
		Route: route,
		Body:  body,
	})
}

func (c *Client) removePending(seq uint64, call *pendingCall) {
	c.mutex.Lock()
	if c.pending[seq] == call {
		delete(c.pending, seq)
	}
	c.mutex.Unlock()
}

func (c *Client) OnPush(route uint32, handler PushHandler) {
	c.mutex.Lock()
	if handler == nil {
		delete(c.pushHandler, route)
	} else {
		c.pushHandler[route] = handler
	}
	c.mutex.Unlock()
}

func (c *Client) HandleMessage(conn transport.Conn, message *packet.Message) {
	if !c.bound.Load() && message.Type != packet.BindRsp {
		_ = conn.Close()
		return
	}

	switch message.Type {
	case packet.BindRsp:
		if c.bound.Load() {
			_ = conn.Close()
			return
		}
		response := &protocol.BindResponse{}
		if err := proto.Unmarshal(message.Body, response); err != nil {
			_ = conn.Close()
			return
		}
		c.mutex.Lock()
		wait := c.bindWait
		if wait == nil {
			c.mutex.Unlock()
			_ = conn.Close()
			return
		}
		if response.Code == protocol.BindCode_BIND_OK {
			if c.closed.Load() {
				c.mutex.Unlock()
				return
			}
			if response.HeartbeatIntervalMilli > 0 {
				c.heartbeat = time.Duration(response.HeartbeatIntervalMilli) * time.Millisecond
			}
			c.bound.Store(true)
		}
		c.mutex.Unlock()
		if response.Code == protocol.BindCode_BIND_OK {
			c.heartbeatRun.Do(func() {
				help.SafeGo(c.heartbeatLoop)
			})
		}
		select {
		case wait <- bindResult{response: response}:
		default:
		}
	case packet.Rsp:
		body, err := decodeBody(c.options, message)
		if err != nil {
			_ = conn.Close()
			return
		}
		c.mutex.Lock()
		call := c.pending[message.Seq]
		if call != nil && call.route == message.Route {
			delete(c.pending, message.Seq)
		}
		c.mutex.Unlock()
		if call == nil {
			return
		}
		if call.route != message.Route {
			_ = conn.Close()
			return
		}
		call.result <- callResult{body: append([]byte(nil), body...)}
	case packet.Push:
		body, err := decodeBody(c.options, message)
		if err != nil {
			_ = conn.Close()
			return
		}
		c.mutex.Lock()
		handler := c.pushHandler[message.Route]
		c.mutex.Unlock()
		if handler != nil {
			help.SafeRun(func() {
				handler(context.Background(), body)
			})
		}
	case packet.Ping:
		if err := conn.Write(&packet.Message{
			Type: packet.Pong,
			Body: message.Body,
		}); err != nil {
			_ = conn.Close()
		}
	case packet.Pong:
		nonce := binary.BigEndian.Uint64(message.Body)
		if c.pingAt.Load() != 0 && nonce == c.pingNonce.Load() {
			c.pingAt.Store(0)
		}
	default:
		_ = conn.Close()
	}
}

func (c *Client) HandleClose(transport.Conn) {
	c.finish()
}

func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(c.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case now := <-ticker.C:
			pingAt := c.pingAt.Load()
			if pingAt != 0 {
				if now.Sub(time.Unix(0, pingAt)) >= c.options.heartbeatTimeout {
					_ = c.Close()
					return
				}
				continue
			}

			var nonceBytes [packet.PingBodySize]byte
			if _, err := rand.Read(nonceBytes[:]); err != nil {
				_ = c.Close()
				return
			}
			c.pingNonce.Store(binary.BigEndian.Uint64(nonceBytes[:]))
			c.pingAt.Store(now.UnixNano())

			c.mutex.Lock()
			conn := c.conn
			c.mutex.Unlock()
			if conn == nil || conn.Write(&packet.Message{
				Type: packet.Ping,
				Body: nonceBytes[:],
			}) != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func (c *Client) finish() {
	c.finishOnce.Do(func() {
		c.closed.Store(true)
		c.bound.Store(false)
		close(c.done)

		c.mutex.Lock()
		wait := c.bindWait
		c.bindWait = nil
		pending := c.pending
		c.pending = make(map[uint64]*pendingCall)
		c.conn = nil
		c.mutex.Unlock()

		if wait != nil {
			select {
			case wait <- bindResult{err: ErrClosed}:
			default:
			}
		}
		for _, call := range pending {
			call.result <- callResult{err: ErrClosed}
		}
	})
}

func (c *Client) Close() error {
	c.mutex.Lock()
	conn := c.conn
	c.mutex.Unlock()
	if conn == nil {
		c.finish()
		return nil
	}
	err := conn.Close()
	c.finish()
	return err
}
