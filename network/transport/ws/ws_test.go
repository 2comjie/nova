package netWs

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"testing/synctest"
	"time"

	"github.com/2comjie/nova/packet"
	"github.com/gorilla/websocket"
)

func TestWriteDeadlineInterruptsWebSocket(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		local, remote := net.Pipe()
		defer remote.Close()
		go func() {
			request, err := http.ReadRequest(bufio.NewReader(remote))
			if err != nil {
				t.Error(err)
				return
			}
			key := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
			_, err = fmt.Fprintf(remote, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(key[:]))
			if err != nil {
				t.Error(err)
			}
			// Keep the peer open without reading frames, to block the actual socket write.
		}()
		raw, _, err := websocket.NewClient(local, &url.URL{Scheme: "ws", Host: "localhost"}, nil, 1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		conn := newConn(raw, nil, false, 4, time.Hour)
		defer conn.Close()
		go conn.writeLoop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := conn.WriteContext(ctx, &packet.Message{Type: packet.Req, Route: 1, Body: []byte("blocked")}); err != context.DeadlineExceeded {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-conn.done:
		default:
			t.Fatal("partially written websocket was not closed")
		}
	})
}
