package network

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/2comjie/nova/network/protocol"
	netTcp "github.com/2comjie/nova/network/transport/tcp"
	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
)

type serverPipeListener struct {
	net.Listener
	accepts chan net.Conn
	done    chan struct{}
	once    sync.Once
}

func (l *serverPipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.accepts:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *serverPipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func newServerTestListener() *serverPipeListener {
	return &serverPipeListener{accepts: make(chan net.Conn), done: make(chan struct{})}
}

func (l *serverPipeListener) connect(t *testing.T) net.Conn {
	local, remote := net.Pipe()
	l.accepts <- local
	t.Cleanup(func() { _ = remote.Close() })
	return remote
}

func writeServerTestPacket(t *testing.T, conn net.Conn, message *packet.Message) {
	t.Helper()
	frame, err := packet.NewCodec(0).Encode(message)
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()
	if _, err := frame.WriteTo(conn); err != nil {
		t.Fatal(err)
	}
}

func readServerTestPacket(t *testing.T, conn net.Conn, kind packet.Type) *packet.Message {
	t.Helper()
	message, err := packet.NewCodec(0).Read(conn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(message.Release)
	if message.Type != kind {
		t.Fatalf("packet type: got %v, want %v", message.Type, kind)
	}
	return message
}

func bindServerTestSession(t *testing.T, conn net.Conn, token string) {
	t.Helper()
	body, err := proto.Marshal(&protocol.BindRequest{Token: []byte(token)})
	if err != nil {
		t.Fatal(err)
	}
	writeServerTestPacket(t, conn, &packet.Message{Type: packet.BindReq, Body: body})
	response := &protocol.BindResponse{}
	if err := proto.Unmarshal(readServerTestPacket(t, conn, packet.BindRsp).Body, response); err != nil {
		t.Fatal(err)
	}
	if response.Code != protocol.BindCode_BIND_OK {
		t.Fatal(response.Code)
	}
}

func TestServerSlowRequestDoesNotBlockHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newServerTestListener()
		entered := make(chan struct{})
		server := NewServer(WithListener(netTcp.NewListener(listener)),
			WithAuther(AuthFunc(func([]byte) (uint64, error) { return 1, nil })),
			WithHeartbeat(10*time.Millisecond, 40*time.Millisecond),
			WithHooks(Hooks{OnReq: func(ctx *ReqContext) { close(entered); <-ctx.Done() }}))
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
		conn := listener.connect(t)
		bindServerTestSession(t, conn, "token")
		writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 1, Seq: 1})
		<-entered
		for range 6 {
			time.Sleep(15 * time.Millisecond)
			writeServerTestPacket(t, conn, &packet.Message{Type: packet.Ping, Body: make([]byte, packet.PingBodySize)})
			readServerTestPacket(t, conn, packet.Pong)
		}
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestServerQueuedBodyIsOwned(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newServerTestListener()
		entered, release := make(chan struct{}), make(chan struct{})
		bodies := make(chan string, 1)
		server := NewServer(WithListener(netTcp.NewListener(listener)), WithRequestQueue(1),
			WithAuther(AuthFunc(func([]byte) (uint64, error) { return 1, nil })),
			WithHooks(Hooks{OnReq: func(ctx *ReqContext) {
				if ctx.Request.Seq == 1 {
					close(entered)
					<-release
				} else {
					bodies <- string(ctx.Request.Body)
				}
			}}))
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
		conn := listener.connect(t)
		bindServerTestSession(t, conn, "token")
		writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 1, Seq: 1})
		<-entered
		writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 1, Seq: 2, Body: []byte("original")})
		synctest.Wait()
		// Control frames reuse the transport's pooled buffers while the request remains queued.
		for range 3 {
			writeServerTestPacket(t, conn, &packet.Message{Type: packet.Ping, Body: []byte("newbytes")})
			readServerTestPacket(t, conn, packet.Pong)
		}
		close(release)
		if body := <-bodies; body != "original" {
			t.Fatal(body)
		}
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestServerRequestQueueIsBounded(t *testing.T) {
	for _, test := range []struct {
		name     string
		capacity int
		bodySize int
	}{{"packet count", 1, 0}, {"total bytes", 128, 2 << 20}} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				listener := newServerTestListener()
				entered, ended := make(chan struct{}), make(chan struct{})
				var calls atomic.Int32
				server := NewServer(WithListener(netTcp.NewListener(listener)), WithRequestQueue(test.capacity),
					WithAuther(AuthFunc(func([]byte) (uint64, error) { return 1, nil })),
					WithHooks(Hooks{
						OnReq:        func(ctx *ReqContext) { calls.Add(1); close(entered); <-ctx.Done() },
						OnSessionEnd: func(context.Context, *Session) { close(ended) },
					}))
				if err := server.Start(); err != nil {
					t.Fatal(err)
				}
				conn := listener.connect(t)
				bindServerTestSession(t, conn, "token")
				writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 1, Seq: 1})
				<-entered
				body := make([]byte, test.bodySize)
				writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 1, Seq: 2, Body: body})
				writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 1, Seq: 3, Body: body})
				<-ended
				if calls.Load() != 1 {
					t.Fatal("queued requests ran after queue overflow closed the connection")
				}
				if err := server.Shutdown(context.Background()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestServerLateOldBindDoesNotReplaceNewSession(t *testing.T) {
	for _, closeOld := range []bool{false, true} {
		name := "late auth"
		if closeOld {
			name = "closed during auth"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				listener := newServerTestListener()
				entered, release := make(chan struct{}), make(chan struct{})
				started := make(chan *Session, 2)
				ended := make(chan *Session, 2)
				server := NewServer(WithListener(netTcp.NewListener(listener)),
					WithAuther(AuthFunc(func(token []byte) (uint64, error) {
						if string(token) == "old" {
							close(entered)
							<-release
						}
						return 1, nil
					})), WithHooks(Hooks{
						OnSessionStart: func(session *Session) { started <- session },
						OnSessionEnd:   func(_ context.Context, session *Session) { ended <- session },
					}))
				if err := server.Start(); err != nil {
					t.Fatal(err)
				}
				old := listener.connect(t)
				oldSession := <-started
				body, _ := proto.Marshal(&protocol.BindRequest{Token: []byte("old")})
				writeServerTestPacket(t, old, &packet.Message{Type: packet.BindReq, Body: body})
				<-entered
				newer := listener.connect(t)
				newSession := <-started
				bindServerTestSession(t, newer, "new")
				if closeOld {
					_ = old.Close()
					synctest.Wait()
				}
				close(release)
				synctest.Wait()
				server.mutex.RLock()
				current := server.byUid[1]
				server.mutex.RUnlock()
				if current != newSession {
					t.Error("late old Bind replaced the newer UID session")
				}
				// A repeated close callback from the old socket must not remove the replacement.
				server.HandleClose(oldSession.Conn)
				server.mutex.RLock()
				current = server.byUid[1]
				server.mutex.RUnlock()
				if current != newSession {
					t.Error("old Close removed the replacement UID session")
				}
				if err := server.Shutdown(context.Background()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestServerShutdownDeadlineReachesAllCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newServerTestListener()
		var entered, completed atomic.Int32
		server := NewServer(WithListener(netTcp.NewListener(listener)),
			WithAuther(AuthFunc(func(token []byte) (uint64, error) { return uint64(token[0]), nil })),
			WithHooks(Hooks{OnSessionEnd: func(ctx context.Context, _ *Session) {
				entered.Add(1)
				<-ctx.Done()
				completed.Add(1)
			}}))
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{"a", "b", "c"} {
			bindServerTestSession(t, listener.connect(t), token)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		err := server.Shutdown(ctx)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if time.Since(start) != time.Second || entered.Load() != 3 {
			t.Fatal("shutdown did not apply one shared deadline to all cleanup")
		}
		synctest.Wait()
		if completed.Load() != 3 {
			t.Fatal("cleanup did not receive cancellation")
		}
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestServerOldBoundSessionCleanupKeepsReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newServerTestListener()
		started, ended := make(chan *Session, 2), make(chan *Session, 2)
		var registered atomic.Uint64
		server := NewServer(WithListener(netTcp.NewListener(listener)),
			WithAuther(AuthFunc(func([]byte) (uint64, error) { return 1, nil })),
			WithHooks(Hooks{
				OnSessionStart: func(session *Session) { started <- session },
				OnSessionBind:  func(session *Session) error { registered.Store(session.Id); return nil },
				OnSessionEnd: func(_ context.Context, session *Session) {
					registered.CompareAndSwap(session.Id, 0)
					ended <- session
				},
			}))
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
		old := listener.connect(t)
		oldSession := <-started
		bindServerTestSession(t, old, "old")
		newer := listener.connect(t)
		newSession := <-started
		bindServerTestSession(t, newer, "new")
		if session := <-ended; session != oldSession {
			t.Fatal("binding replacement did not close the old session")
		}
		server.HandleClose(oldSession.Conn)
		server.mutex.RLock()
		current := server.byUid[1]
		server.mutex.RUnlock()
		if current != newSession || registered.Load() != newSession.Id {
			t.Fatal("old session cleanup removed its replacement")
		}
		result := make(chan error, 1)
		go func() { result <- server.PushUid(context.Background(), 1, 10, []byte("new session")) }()
		message := readServerTestPacket(t, newer, packet.Push)
		if string(message.Body) != "new session" {
			t.Fatal("push did not reach the replacement")
		}
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestServerShutdownWaitsForNormalCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newServerTestListener()
		entered, release := make(chan struct{}), make(chan struct{})
		server := NewServer(WithListener(netTcp.NewListener(listener)),
			WithAuther(AuthFunc(func([]byte) (uint64, error) { return 1, nil })),
			WithHooks(Hooks{OnSessionEnd: func(ctx context.Context, _ *Session) {
				if err := ctx.Err(); err != nil {
					t.Errorf("cleanup already canceled: %v", err)
				}
				close(entered)
				<-release
			}}))
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
		bindServerTestSession(t, listener.connect(t), "token")
		result := make(chan error, 1)
		go func() { result <- server.Shutdown(context.Background()) }()
		<-entered
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("shutdown returned before cleanup finished: %v", err)
		default:
		}
		close(release)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	})
}

func TestServerRequestDeadlineCanSendCustomResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newServerTestListener()
		server := NewServer(WithListener(netTcp.NewListener(listener)), WithRequestTimeout(time.Second),
			WithAuther(AuthFunc(func([]byte) (uint64, error) { return 1, nil })),
			WithHooks(Hooks{OnReq: func(ctx *ReqContext) {
				<-ctx.Done()
				if ctx.Err() != context.DeadlineExceeded {
					t.Errorf("request context: %v", ctx.Err())
				}
				if err := ctx.Write([]byte("timed out")); err != nil {
					t.Errorf("timeout response: %v", err)
				}
			}}))
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
		conn := listener.connect(t)
		bindServerTestSession(t, conn, "token")
		start := time.Now()
		writeServerTestPacket(t, conn, &packet.Message{Type: packet.Req, Route: 7, Seq: 9})
		message := readServerTestPacket(t, conn, packet.Rsp)
		if time.Since(start) != time.Second || message.Route != 7 || message.Seq != 9 || string(message.Body) != "timed out" {
			t.Fatal("request deadline did not produce its correlated timeout response")
		}
		writeServerTestPacket(t, conn, &packet.Message{Type: packet.Ping, Body: make([]byte, packet.PingBodySize)})
		readServerTestPacket(t, conn, packet.Pong)
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}
