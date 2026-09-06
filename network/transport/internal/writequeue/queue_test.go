package writequeue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/2comjie/nova/core/buffer"
	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
)

func TestQueuedCancellationSkipsFrame(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(packet.NewCodec(0), 4, time.Second)
		defer q.Close()
		entered, release := make(chan struct{}), make(chan struct{})
		var sent []string
		go func() {
			q.Run(func(frame *buffer.Bytes, _ time.Time) error {
				if len(sent) == 0 {
					close(entered)
					<-release
				}
				sent = append(sent, string(frame.Bytes()[packet.HeaderSize:]))
				return nil
			}, func() { t.Error("queued cancellation interrupted the active write") })
		}()
		first := make(chan error, 1)
		go func() {
			first <- q.Write(context.Background(), &packet.Message{Type: packet.Req, Route: 1, Body: []byte("first")})
		}()
		<-entered
		ctx, cancel := context.WithCancel(context.Background())
		second := make(chan error, 1)
		go func() { second <- q.Write(ctx, &packet.Message{Type: packet.Req, Route: 1, Body: []byte("canceled")}) }()
		synctest.Wait()
		cancel()
		if err := <-second; err != context.Canceled {
			t.Fatal(err)
		}
		third := make(chan error, 1)
		go func() {
			third <- q.Write(context.Background(), &packet.Message{Type: packet.Req, Route: 1, Body: []byte("third")})
		}()
		synctest.Wait()
		close(release)
		if err := <-first; err != nil {
			t.Fatal(err)
		}
		if err := <-third; err != nil {
			t.Fatal(err)
		}
		if len(sent) != 2 || sent[0] != "first" || sent[1] != "third" {
			t.Fatal(sent)
		}
	})
}

func TestActiveCancellationOwnsFrameAndInterruptsWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(packet.NewCodec(0), 4, time.Second)
		defer q.Close()
		entered, interrupted, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		finished := make(chan struct{})
		go func() {
			q.Run(func(frame *buffer.Bytes, _ time.Time) error {
				close(entered)
				<-release
				if string(frame.Bytes()[packet.HeaderSize:]) != "original" {
					t.Error("writer accessed the caller's reused body")
				}
				return errors.New("interrupted write")
			}, func() { close(interrupted) })
			close(finished)
		}()
		ctx, cancel := context.WithCancel(context.Background())
		body := []byte("original")
		result := make(chan error, 1)
		go func() { result <- q.Write(ctx, &packet.Message{Type: packet.Req, Route: 1, Body: body}) }()
		<-entered
		cancel()
		if err := <-result; err != context.Canceled {
			t.Fatal(err)
		}
		clear(body)
		<-interrupted
		close(release)
		<-finished
	})
}

func TestCloseReleasesQueuedFrames(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(packet.NewCodec(0), 4, time.Second)
		var wait sync.WaitGroup
		for range 4 {
			wait.Go(func() {
				if err := q.Write(context.Background(), &packet.Message{Type: packet.Req, Route: 1}); err != transport.ErrClosed {
					t.Errorf("Write: %v", err)
				}
			})
		}
		synctest.Wait()
		q.Close()
		wait.Wait()
		if q.queuedBytes != 0 || len(q.writes) != 0 {
			t.Fatal("closed queue retains frames")
		}
	})
}

func TestQueueLimitAndConcurrentClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(packet.NewCodec(0), 1, time.Second)
		result := make(chan error, 1)
		go func() { result <- q.Write(context.Background(), &packet.Message{Type: packet.Req, Route: 1}) }()
		synctest.Wait()
		if err := q.Write(context.Background(), &packet.Message{Type: packet.Req, Route: 1}); err != transport.ErrWriteQueueFull {
			t.Fatal(err)
		}
		q.Close()
		if err := <-result; err != transport.ErrClosed {
			t.Fatal(err)
		}
	})
	q := New(packet.NewCodec(0), 32, time.Second)
	var wait sync.WaitGroup
	for range 32 {
		wait.Go(func() {
			if err := q.Write(context.Background(), &packet.Message{Type: packet.Req, Route: 1}); err != transport.ErrClosed {
				t.Errorf("Write during close: %v", err)
			}
		})
	}
	q.Close()
	wait.Wait()
	if q.queuedBytes != 0 || len(q.writes) != 0 {
		t.Fatal("concurrent close retains frames")
	}
}
