package network

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/2comjie/nova/network/protocol"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

type clientTestConn struct {
	transport.Conn
	started atomic.Bool
	closed  atomic.Bool
	write   func(context.Context, *packet.Message) error
}

func (c *clientTestConn) Start(transport.Handler) error { c.started.Store(true); return nil }
func (c *clientTestConn) Close() error                  { c.closed.Store(true); return nil }
func (c *clientTestConn) WriteContext(ctx context.Context, message *packet.Message) error {
	if c.write != nil {
		return c.write(ctx, message)
	}
	<-ctx.Done()
	return ctx.Err()
}

type clientTestDialer func(context.Context) (transport.Conn, error)

func (d clientTestDialer) DialContext(ctx context.Context) (transport.Conn, error) { return d(ctx) }

func TestNewClientRequiresDialer(t *testing.T) {
	defer func() {
		if value := recover(); value != ErrDialerMissing {
			t.Fatalf("panic=%v, want %v", value, ErrDialerMissing)
		}
	}()
	NewClient()
}

func TestCloseDuringDialClosesReturnedConnection(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	conn := &clientTestConn{}
	client := NewClient(WithDialer(clientTestDialer(func(context.Context) (transport.Conn, error) {
		close(entered)
		<-release
		return conn, nil
	})))
	result := make(chan error, 1)
	go func() { result <- client.Dial(context.Background()) }()
	<-entered
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; err != ErrClosed {
		t.Fatal(err)
	}
	if !conn.closed.Load() || conn.started.Load() || client.conn != nil {
		t.Fatal("closed client installed or started a new connection")
	}
	select {
	case <-client.Done():
	default:
		t.Fatal("Done remains open after Close")
	}
}

func TestClientRequestsUseWriteContext(t *testing.T) {
	for _, method := range []string{"Call", "Tell", "Bind"} {
		t.Run(method, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				conn := &clientTestConn{}
				client := NewClient(WithDialer(clientTestDialer(func(context.Context) (transport.Conn, error) { return conn, nil })))
				if err := client.Dial(context.Background()); err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				client.bound.Store(method != "Bind")
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var err error
				switch method {
				case "Call":
					_, err = client.Call(ctx, 1, nil)
				case "Tell":
					err = client.Tell(ctx, 1, nil)
				case "Bind":
					err = client.Bind(ctx, []byte("token"))
				}
				if err != context.DeadlineExceeded {
					t.Fatal(err)
				}
				if len(client.pending) != 0 || client.bindWait != nil {
					t.Fatal("canceled send remains pending")
				}
			})
		})
	}
}

func TestClientResponseOwnsBody(t *testing.T) {
	conn := &clientTestConn{}
	client := NewClient(WithDialer(clientTestDialer(func(context.Context) (transport.Conn, error) { return conn, nil })))
	client.bound.Store(true)
	call := &pendingCall{route: 7, result: make(chan callResult, 1)}
	client.pending[1] = call
	body := []byte("response")
	client.HandleMessage(conn, &packet.Message{Type: packet.Rsp, Route: 7, Seq: 1, Body: body})
	clear(body)
	result := <-call.result
	if result.err != nil || string(result.body) != "response" {
		t.Fatalf("response: %q, %v", result.body, result.err)
	}
	if len(client.pending) != 0 || conn.closed.Load() {
		t.Fatal("reply closed the connection or left a pending request")
	}
}

func TestCloseUnblocksPendingRequests(t *testing.T) {
	for _, method := range []string{"Call", "Bind"} {
		t.Run(method, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				conn := &clientTestConn{write: func(context.Context, *packet.Message) error { return nil }}
				client := NewClient(WithDialer(clientTestDialer(func(context.Context) (transport.Conn, error) { return conn, nil })))
				if err := client.Dial(context.Background()); err != nil {
					t.Fatal(err)
				}
				count := 1
				if method == "Call" {
					client.bound.Store(true)
					count = 4
				}
				results := make(chan error, count)
				for range count {
					go func() {
						if method == "Bind" {
							results <- client.Bind(context.Background(), []byte("token"))
						} else {
							_, err := client.Call(context.Background(), 7, nil)
							results <- err
						}
					}()
				}
				synctest.Wait()
				pending, bindWait := client.pending, client.bindWait
				if method == "Call" && len(pending) != count || method == "Bind" && bindWait == nil {
					t.Fatal("requests did not start waiting for responses")
				}
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				for range count {
					if err := <-results; err != ErrClosed {
						t.Fatalf("pending %s returned %v, want ErrClosed", method, err)
					}
				}
				if client.pending != nil || client.bindWait != nil || client.conn != nil {
					t.Fatal("closed client retained pending request or connection references")
				}
				if len(bindWait) != 0 {
					t.Fatal("Close sent a redundant bind result")
				}
				for _, call := range pending {
					if len(call.result) != 0 {
						t.Fatal("Close sent a redundant call result")
					}
				}
			})
		})
	}
}

func TestCloseRacesWithResponse(t *testing.T) {
	for _, method := range []string{"Call", "Bind"} {
		t.Run(method, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				conn := &clientTestConn{write: func(context.Context, *packet.Message) error { return nil }}
				client := NewClient(WithDialer(clientTestDialer(func(context.Context) (transport.Conn, error) { return conn, nil })))
				if err := client.Dial(context.Background()); err != nil {
					t.Fatal(err)
				}
				client.bound.Store(method == "Call")
				result := make(chan callResult, 1)
				go func() {
					if method == "Bind" {
						result <- callResult{err: client.Bind(context.Background(), []byte("token"))}
					} else {
						body, err := client.Call(context.Background(), 7, nil)
						result <- callResult{body: body, err: err}
					}
				}()
				synctest.Wait()
				response := &packet.Message{Type: packet.Rsp, Route: 7, Seq: 1, Body: []byte("ok")}
				if method == "Bind" {
					body, err := proto.Marshal(&protocol.BindResponse{Code: protocol.BindCode_BIND_OK})
					if err != nil {
						t.Fatal(err)
					}
					response = &packet.Message{Type: packet.BindRsp, Body: body}
				}
				go client.HandleMessage(conn, response)
				go client.Close()
				synctest.Wait()
				got := <-result
				if got.err != nil && got.err != ErrClosed {
					t.Fatalf("response racing with Close returned %v", got.err)
				}
				if method == "Call" && got.err == nil && string(got.body) != "ok" {
					t.Fatalf("successful response body=%q", got.body)
				}
				if client.bound.Load() || client.pending != nil || client.bindWait != nil || client.conn != nil {
					t.Fatal("response revived the closed client or retained request references")
				}
			})
		})
	}
}
