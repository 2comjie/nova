package rpc

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/nova/packet"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func startTestServer(t *testing.T, server *Server, opts ...ConnOption) *Conn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil && err != ErrClosed {
			t.Error(err)
		}
	})
	conn := NewConn(listener.Addr().String(), opts...)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestConcurrentCallsAndSingleConnection(t *testing.T) {
	server := NewServer()
	entered, release := make(chan struct{}), make(chan struct{})
	server.Register("echo", func(ctx context.Context, body []byte) (proto.Message, *Error) {
		request := new(wrapperspb.Int64Value)
		if err := proto.Unmarshal(body, request); err != nil {
			return nil, FromError(err)
		}
		if request.Value == -1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, FromError(ctx.Err())
			}
		}
		return request, nil
	})
	conn := startTestServer(t, server)
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	blocked := make(chan error, 1)
	go func() { blocked <- conn.Invoke(t.Context(), "echo", wrapperspb.Int64(-1), new(wrapperspb.Int64Value)) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	var calls sync.WaitGroup
	for index := range 100 {
		calls.Go(func() {
			response := new(wrapperspb.Int64Value)
			if err := conn.Invoke(t.Context(), "echo", wrapperspb.Int64(int64(index)), response); err != nil || response.Value != int64(index) {
				t.Errorf("call %d: response=%v err=%v", index, response, err)
			}
		})
	}
	calls.Wait()
	server.mu.Lock()
	count := len(server.connections)
	server.mu.Unlock()
	if count != 1 {
		t.Fatalf("connections=%d, want 1", count)
	}
	select {
	case err := <-blocked:
		t.Fatalf("blocked call returned early: %v", err)
	default:
	}
	unblock.Do(func() { close(release) })
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
}

func TestBusinessErrorAndRegistrationOverwrite(t *testing.T) {
	server := NewServer()
	server.Register("failure", func(context.Context, []byte) (proto.Message, *Error) { panic("replaced handler") })
	want := NewErrorWithDetail(1001, "not enough coins", []byte("details"))
	server.Register("failure", func(context.Context, []byte) (proto.Message, *Error) { return nil, want })
	conn := startTestServer(t, server)
	for _, method := range []string{"failure", "missing"} {
		err := conn.Invoke(t.Context(), method, &emptypb.Empty{}, &emptypb.Empty{})
		var failure *Error
		if !errors.As(err, &failure) {
			t.Fatalf("error=%v", err)
		}
		if method == "failure" && !proto.Equal(failure, want) {
			t.Fatalf("business error=%v", failure)
		}
		if method == "missing" && failure.Code != CodeNotFound {
			t.Fatalf("missing method=%v", failure)
		}
	}
}

func TestCancellationIgnoresLateReply(t *testing.T) {
	server := NewServer()
	entered, release, replied := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server.Register("late", func(context.Context, []byte) (proto.Message, *Error) {
		close(entered)
		<-release
		close(replied)
		return wrapperspb.String("late"), nil
	})
	server.Register("echo", func(context.Context, []byte) (proto.Message, *Error) { return wrapperspb.String("next"), nil })
	conn := startTestServer(t, server)
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- conn.Invoke(ctx, "late", &emptypb.Empty{}, new(wrapperspb.StringValue)) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call=%v", err)
	}
	conn.mu.Lock()
	pending := len(conn.pending)
	conn.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending=%d", pending)
	}
	unblock.Do(func() { close(release) })
	<-replied
	response := new(wrapperspb.StringValue)
	if err := conn.Invoke(t.Context(), "echo", &emptypb.Empty{}, response); err != nil || response.Value != "next" {
		t.Fatalf("next response=%v err=%v", response, err)
	}
}

func TestDisconnectCancelsCallWithoutReplay(t *testing.T) {
	server := NewServer()
	entered, canceled := make(chan struct{}), make(chan struct{})
	var executions atomic.Int32
	server.Register("blocked", func(ctx context.Context, _ []byte) (proto.Message, *Error) {
		executions.Add(1)
		close(entered)
		<-ctx.Done()
		close(canceled)
		return nil, FromError(ctx.Err())
	})
	server.Register("echo", func(context.Context, []byte) (proto.Message, *Error) { return &emptypb.Empty{}, nil })
	conn := startTestServer(t, server)
	result := make(chan error, 1)
	go func() { result <- conn.Invoke(t.Context(), "blocked", &emptypb.Empty{}, &emptypb.Empty{}) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	conn.mu.Lock()
	raw := conn.conn
	conn.mu.Unlock()
	_ = raw.Close()
	if err := <-result; err != ErrClosed {
		t.Fatalf("disconnect=%v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("server handler was not canceled")
	}
	if err := conn.Invoke(t.Context(), "echo", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 1 {
		t.Fatalf("request replayed %d times", executions.Load())
	}
}

func TestSendDoesNotWaitForBusinessReply(t *testing.T) {
	server := NewServer()
	entered, release := make(chan struct{}), make(chan struct{})
	server.Register("send", func(ctx context.Context, _ []byte) (proto.Message, *Error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, nil
	})
	conn := startTestServer(t, server)
	defer close(release)
	if err := conn.Send(t.Context(), "send", &emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Send was not delivered")
	}
	conn.mu.Lock()
	pending := len(conn.pending)
	conn.mu.Unlock()
	if pending != 0 {
		t.Fatalf("Send created %d pending calls", pending)
	}
}

func TestPendingAndExecutionLimits(t *testing.T) {
	for _, clientLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "server", true: "client"}[clientLimit], func(t *testing.T) {
			server := NewServer(WithMaxConcurrent(1))
			entered, release := make(chan struct{}), make(chan struct{})
			server.Register("blocked", func(ctx context.Context, _ []byte) (proto.Message, *Error) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, FromError(ctx.Err())
				}
				return &emptypb.Empty{}, nil
			})
			var opts []ConnOption
			if clientLimit {
				opts = append(opts, WithMaxPending(1))
			}
			conn := startTestServer(t, server, opts...)
			var unblock sync.Once
			defer unblock.Do(func() { close(release) })
			result := make(chan error, 1)
			go func() { result <- conn.Invoke(t.Context(), "blocked", &emptypb.Empty{}, &emptypb.Empty{}) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request did not start")
			}
			err := conn.Invoke(t.Context(), "blocked", &emptypb.Empty{}, &emptypb.Empty{})
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != CodeBusy {
				t.Fatalf("overload error=%v", err)
			}
			unblock.Do(func() { close(release) })
			if err := <-result; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGracefulShutdownWaitsForResponse(t *testing.T) {
	server := NewServer()
	entered, release := make(chan struct{}), make(chan struct{})
	server.Register("wait", func(ctx context.Context, _ []byte) (proto.Message, *Error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, FromError(ctx.Err())
		}
		return wrapperspb.String("done"), nil
	})
	conn := startTestServer(t, server)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	response := new(wrapperspb.StringValue)
	result := make(chan error, 1)
	go func() { result <- conn.Invoke(t.Context(), "wait", &emptypb.Empty{}, response) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	shutdown := make(chan error, 1)
	go func() { shutdown <- server.Shutdown(t.Context()) }()
	select {
	case err := <-shutdown:
		t.Fatalf("shutdown returned early: %v", err)
	default:
	}
	close(release)
	if err := <-result; err != nil || response.Value != "done" {
		t.Fatalf("response=%v err=%v", response, err)
	}
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
}

func TestRequestDeadline(t *testing.T) {
	server := NewServer()
	serverDeadline := make(chan error, 1)
	server.Register("wait", func(ctx context.Context, _ []byte) (proto.Message, *Error) {
		<-ctx.Done()
		serverDeadline <- ctx.Err()
		return nil, FromError(ctx.Err())
	})
	conn := startTestServer(t, server, WithCallTimeout(100*time.Millisecond))
	err := conn.Invoke(t.Context(), "wait", &emptypb.Empty{}, &emptypb.Empty{})
	var failure *Error
	if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &failure) && failure.Code == CodeDeadlineExceeded) {
		t.Fatalf("deadline error=%v", err)
	}
	select {
	case err := <-serverDeadline:
		if err != context.DeadlineExceeded {
			t.Fatalf("server context=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline was not propagated to the server")
	}
}

func TestShutdownDeadlineCancelsHandler(t *testing.T) {
	server := NewServer()
	entered, canceled := make(chan struct{}), make(chan struct{})
	server.Register("wait", func(ctx context.Context, _ []byte) (proto.Message, *Error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return nil, FromError(ctx.Err())
	})
	conn := startTestServer(t, server)
	result := make(chan error, 1)
	go func() { result <- conn.Invoke(t.Context(), "wait", &emptypb.Empty{}, &emptypb.Empty{}) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); err != context.DeadlineExceeded {
		t.Fatalf("shutdown error=%v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the handler")
	}
	if err := <-result; err != ErrClosed {
		t.Fatalf("pending call error=%v", err)
	}
}

func TestCanceledCallDoesNotWaitForDial(t *testing.T) {
	conn := NewConn("127.0.0.1:1")
	conn.dial <- struct{}{}
	defer func() { <-conn.dial }()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := conn.Invoke(ctx, "unused", &emptypb.Empty{}, &emptypb.Empty{}); err != context.Canceled {
		t.Fatalf("canceled dial=%v", err)
	}
}

func TestServerRejectsMalformedEnvelope(t *testing.T) {
	server := NewServer()
	conn := startTestServer(t, server)
	raw, err := net.Dial("tcp", conn.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	frame, err := packet.NewCodec(packet.DefaultMaxFrame).Encode(&packet.Message{Type: packet.Req, Route: 1, Seq: 1, Body: []byte{0xff}})
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()
	if _, err := raw.Write(frame.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := raw.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var body [1]byte
	_, err = raw.Read(body[:])
	if err == nil {
		t.Fatal("malformed request was accepted")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("malformed connection was not closed")
	}
}
