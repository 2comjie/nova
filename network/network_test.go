package network_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/wali/network"
	"github.com/2comjie/wali/network/transport"
	kcptransport "github.com/2comjie/wali/network/transport/kcp"
	tcptransport "github.com/2comjie/wali/network/transport/tcp"
	wstransport "github.com/2comjie/wali/network/transport/ws"
)

type testZipper struct{}

func (testZipper) Zip(_ uint32, body []byte) ([]byte, error) {
	return append([]byte{'z'}, body...), nil
}

func (testZipper) Unzip(_ uint32, body []byte) ([]byte, error) {
	if len(body) == 0 || body[0] != 'z' {
		return nil, errors.New("压缩格式错误")
	}
	return body[1:], nil
}

type testCryptor struct{}

func (testCryptor) Encrypt(_ uint32, _ uint64, body []byte) ([]byte, error) {
	result := append([]byte(nil), body...)
	for index := range result {
		result[index] ^= 0x5a
	}
	return result, nil
}

func (testCryptor) Decrypt(route uint32, seq uint64, body []byte) ([]byte, error) {
	return testCryptor{}.Encrypt(route, seq, body)
}

func TestServerClientLifecycle(t *testing.T) {
	listener, err := tcptransport.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan *network.Session, 1)
	bind := make(chan *network.Session, 1)
	end := make(chan *network.Session, 1)
	heartbeat := make(chan struct{}, 1)
	reqBody := make(chan string, 1)
	server, err := network.NewServer(
		network.WithListener(listener),
		network.WithAuther(network.AuthFunc(func(token []byte) (string, error) {
			if !bytes.Equal(token, []byte("valid-token")) {
				return "", network.ErrUnauthorized
			}
			return "user-1", nil
		})),
		network.WithZipper(testZipper{}),
		network.WithCryptor(testCryptor{}),
		network.WithHeartbeat(20*time.Millisecond, 200*time.Millisecond),
		network.WithHooks(network.Hooks{
			OnSessionStart: func(session *network.Session) {
				start <- session
			},
			OnSessionBind: func(session *network.Session) {
				bind <- session
			},
			OnSessionEnd: func(session *network.Session) {
				end <- session
			},
			OnHeartbeat: func(*network.Session) {
				select {
				case heartbeat <- struct{}{}:
				default:
				}
			},
			OnReq: func(ctx *network.ReqContext) {
				if ctx.Request.Route == 10 {
					reqBody <- string(ctx.Request.Body)
					_ = ctx.Write(append([]byte("rsp:"), ctx.Request.Body...))
				}
			},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())

	client, err := network.NewClient(
		network.WithDialer(tcptransport.NewDialer(listener.Addr().String())),
		network.WithZipper(testZipper{}),
		network.WithCryptor(testCryptor{}),
		network.WithHeartbeat(20*time.Millisecond, 200*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	push := make(chan string, 1)
	client.OnPush(11, func(_ context.Context, body []byte) {
		push <- string(body)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Dial(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Bind(ctx, []byte("valid-token")); err != nil {
		t.Fatal(err)
	}

	session := waitValue(t, ctx, bind)
	if session.ID == 0 || session.UID() != "user-1" || !session.IsBound() || session.BoundAt().IsZero() {
		t.Fatalf("Bind后的Session不正确: uid=%q bound=%v", session.UID(), session.IsBound())
	}
	startedSession := waitValue(t, ctx, start)
	if startedSession != session {
		t.Fatal("SessionStart和SessionBind不是同一个Session")
	}

	response, err := client.Call(ctx, 10, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "rsp:hello" {
		t.Fatalf("Rsp错误: %q", response)
	}
	if body := waitValue(t, ctx, reqBody); body != "hello" {
		t.Fatalf("OnReq收到的不是解密解压后的Body: %q", body)
	}

	noResponseCtx, noResponseCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer noResponseCancel()
	if _, err := client.Call(noResponseCtx, 12, []byte("no-response")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("OnReq未Write时不应返回Rsp: %v", err)
	}

	if err := server.PushUID(ctx, "user-1", 11, []byte("notice")); err != nil {
		t.Fatal(err)
	}
	if body := waitValue(t, ctx, push); body != "notice" {
		t.Fatalf("Push错误: %q", body)
	}
	waitValue(t, ctx, heartbeat)

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if ended := waitValue(t, ctx, end); ended != session {
		t.Fatal("SessionEnd收到错误Session")
	}
}

func TestUnboundConnectionExpires(t *testing.T) {
	listener, err := tcptransport.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var ended atomic.Int32
	server, err := network.NewServer(
		network.WithListener(listener),
		network.WithAuther(network.AuthFunc(func([]byte) (string, error) {
			return "unused", nil
		})),
		network.WithBindTimeout(30*time.Millisecond),
		network.WithHooks(network.Hooks{
			OnSessionEnd: func(*network.Session) {
				ended.Add(1)
			},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())

	raw, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := raw.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var one [1]byte
	if _, err := raw.Read(one[:]); err == nil {
		t.Fatal("未Bind连接没有按时关闭")
	}

	deadline := time.Now().Add(time.Second)
	for ended.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ended.Load() != 1 {
		t.Fatal("未触发SessionEnd")
	}
}

func TestDuplicateUIDReplacesOldSession(t *testing.T) {
	listener, err := tcptransport.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan uint64, 2)
	server, err := network.NewServer(
		network.WithListener(listener),
		network.WithAuther(network.AuthFunc(func([]byte) (string, error) {
			return "same-user", nil
		})),
		network.WithHooks(network.Hooks{
			OnSessionEnd: func(session *network.Session) {
				ended <- session.ID
			},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first, err := network.NewClient(network.WithDialer(tcptransport.NewDialer(listener.Addr().String())))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := first.Dial(ctx); err != nil {
		t.Fatal(err)
	}
	if err := first.Bind(ctx, []byte("first")); err != nil {
		t.Fatal(err)
	}

	second, err := network.NewClient(network.WithDialer(tcptransport.NewDialer(listener.Addr().String())))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	push := make(chan string, 1)
	second.OnPush(9, func(_ context.Context, body []byte) {
		push <- string(body)
	})
	if err := second.Dial(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Bind(ctx, []byte("second")); err != nil {
		t.Fatal(err)
	}

	waitValue(t, ctx, ended)
	if err := server.PushUID(ctx, "same-user", 9, []byte("new-session")); err != nil {
		t.Fatal(err)
	}
	if body := waitValue(t, ctx, push); body != "new-session" {
		t.Fatalf("消息没有推送到新Session: %q", body)
	}
}

func TestTransportMatrix(t *testing.T) {
	tests := []struct {
		name   string
		listen func() (transport.Listener, error)
		dial   func(transport.Listener) transport.Dialer
	}{
		{
			name: "tcp",
			listen: func() (transport.Listener, error) {
				return tcptransport.Listen("127.0.0.1:0")
			},
			dial: func(listener transport.Listener) transport.Dialer {
				return tcptransport.NewDialer(listener.Addr().String())
			},
		},
		{
			name: "kcp",
			listen: func() (transport.Listener, error) {
				return kcptransport.Listen("127.0.0.1:0")
			},
			dial: func(listener transport.Listener) transport.Dialer {
				return kcptransport.NewDialer(listener.Addr().String())
			},
		},
		{
			name: "ws",
			listen: func() (transport.Listener, error) {
				return wstransport.Listen("127.0.0.1:0")
			},
			dial: func(listener transport.Listener) transport.Dialer {
				return wstransport.NewDialer("ws://" + listener.Addr().String() + "/")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			listener, err := test.listen()
			if err != nil {
				t.Fatal(err)
			}
			server, err := network.NewServer(
				network.WithListener(listener),
				network.WithAuther(network.AuthFunc(func([]byte) (string, error) {
					return test.name, nil
				})),
				network.WithHooks(network.Hooks{
					OnReq: func(ctx *network.ReqContext) {
						_ = ctx.Write(ctx.Request.Body)
					},
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := server.Start(); err != nil {
				t.Fatal(err)
			}
			defer server.Shutdown(context.Background())

			client, err := network.NewClient(network.WithDialer(test.dial(listener)))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.Dial(ctx); err != nil {
				t.Fatal(err)
			}
			if err := client.Bind(ctx, []byte("token")); err != nil {
				t.Fatal(err)
			}
			response, err := client.Call(ctx, 1, []byte(test.name))
			if err != nil {
				t.Fatal(err)
			}
			if string(response) != test.name {
				t.Fatalf("响应错误: %q", response)
			}
		})
	}
}

func TestTCPWithTLS(t *testing.T) {
	certificate, roots := testCertificate(t)
	listener, err := tcptransport.Listen(
		"127.0.0.1:0",
		tcptransport.WithTLS(&tls.Config{Certificates: []tls.Certificate{certificate}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	secure := make(chan bool, 1)
	server, err := network.NewServer(
		network.WithListener(listener),
		network.WithAuther(network.AuthFunc(func([]byte) (string, error) {
			return "tls-user", nil
		})),
		network.WithHooks(network.Hooks{
			OnSessionBind: func(session *network.Session) {
				secure <- session.Conn.Secure()
			},
			OnReq: func(ctx *network.ReqContext) {
				_ = ctx.Write(ctx.Request.Body)
			},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())

	client, err := network.NewClient(network.WithDialer(tcptransport.NewDialer(
		listener.Addr().String(),
		tcptransport.WithTLS(&tls.Config{
			RootCAs:    roots,
			ServerName: "localhost",
		}),
	)))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Dial(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Bind(ctx, []byte("token")); err != nil {
		t.Fatal(err)
	}
	if !waitValue(t, ctx, secure) {
		t.Fatal("TLS连接未标记为安全连接")
	}
	response, err := client.Call(ctx, 1, []byte("tls"))
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "tls" {
		t.Fatalf("TLS响应错误: %q", response)
	}
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return certificate, roots
}

func waitValue[T any](t *testing.T, ctx context.Context, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-ctx.Done():
		var zero T
		t.Fatal(ctx.Err())
		return zero
	}
}
