package netconn

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/2comjie/nova/network/transport"
	"github.com/2comjie/nova/packet"
)

func TestWriteDeadlineInterruptsBlockedSocket(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		local, remote := net.Pipe()
		defer remote.Close()
		conn := New(local, nil, transport.TypeTCP, false, 4, time.Hour)
		defer conn.Close()
		go conn.writeLoop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		started := time.Now()
		if err := conn.WriteContext(ctx, &packet.Message{Type: packet.Req, Route: 1, Body: []byte("blocked")}); err != context.DeadlineExceeded {
			t.Fatal(err)
		}
		if time.Since(started) != time.Second {
			t.Fatal("write did not honor its request deadline")
		}
		synctest.Wait()
		select {
		case <-conn.done:
		default:
			t.Fatal("partially written connection was not closed")
		}
	})
}
