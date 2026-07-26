package gate

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/network"
	"github.com/2comjie/wali/network/protocol"
	"github.com/2comjie/wali/network/transport"
	"github.com/2comjie/wali/packet"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGateCallTellPushKick(t *testing.T) {
	router := NewRouter()
	if err := router.RegisterFilter("rewrite", func(map[string]string) (Filter, error) {
		return func(ctx *Context, next Handler) error {
			if ctx.App == nil || ctx.App.Instance().ID != "gate-1" {
				return errors.New("Gate Context没有正确设置App Proxy")
			}
			ctx.Route = 110
			return next(ctx)
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(
		Route{
			ID:      "call",
			Routes:  []uint32{10},
			Filters: []FilterConfig{{Name: "rewrite"}},
			Target:  Target{Mode: RouteModeBalance, Service: "player"},
		},
		Route{
			ID:     "tell",
			Routes: []uint32{11},
			Target: Target{Mode: RouteModeSelect, Service: "player"},
		},
	); err != nil {
		t.Fatal(err)
	}

	nodeClient := &testNodeClient{
		response: &pbNode.Response{
			Replied:         true,
			Body:            []byte("node-response"),
			NodeServiceName: "player",
			NodeInstanceId:  "player-1",
		},
	}
	locatorProvider := newTestLocatorProvider()
	gateLocator := locator.NewGateLocator(locatorProvider, nil)
	serviceRegistry := &testRegistry{}
	listener := newTestListener()
	conn := newTestConn()
	listener.connections <- conn
	errorChannel := make(chan error, 1)
	rpcServer := grpc.NewServer()
	rpcListener := bufconn.Listen(1 << 20)
	var hookSessionStart atomic.Int32
	var hookSessionBind atomic.Int32
	var hookSessionEnd atomic.Int32
	var hookHeartbeat atomic.Int32
	var hookReq atomic.Int32

	g, err := New(Config{
		Instance: endpoint.ServiceInstance{
			ID:          "gate-1",
			ServiceName: "gate",
			RpcHost:     "127.0.0.1",
			RpcPort:     9001,
			Status:      endpoint.Working,
		},
		Router:      router,
		NodeClient:  nodeClient,
		Locator:     gateLocator,
		Registry:    serviceRegistry,
		RPCServer:   rpcServer,
		RPCListener: rpcListener,
		Hooks: network.Hooks{
			OnSessionStart: func(*network.Session) {
				hookSessionStart.Add(1)
			},
			OnSessionBind: func(*network.Session) {
				hookSessionBind.Add(1)
			},
			OnSessionEnd: func(*network.Session) {
				hookSessionEnd.Add(1)
			},
			OnHeartbeat: func(*network.Session) {
				hookHeartbeat.Add(1)
			},
			OnReq: func(*network.ReqContext) {
				hookReq.Add(1)
			},
		},
		NetworkOptions: []network.Option{
			network.WithListener(listener),
			network.WithAuther(network.AuthFunc(func(token []byte) (string, error) {
				if string(token) != "token" {
					return "", errors.New("invalid token")
				}
				return "user-1", nil
			})),
		},
		ErrorHandler: func(ctx *Context, err error) {
			errorChannel <- err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = g.Shutdown(context.Background())
	})

	select {
	case <-conn.started:
	case <-time.After(time.Second):
		t.Fatal("连接没有启动")
	}
	bindSession(t, conn)
	if got := locatorProvider.locate("user-1"); got != "gate-1" {
		t.Fatalf("GateLocator=%q, want gate-1", got)
	}
	conn.send(&packet.Message{Type: packet.Ping, Body: make([]byte, 8)})
	if pong := nextMessage(t, conn); pong.Type != packet.Pong {
		t.Fatalf("心跳响应类型=%d, want Pong", pong.Type)
	}

	conn.send(&packet.Message{
		Type:  packet.Req,
		Route: 10,
		Seq:   1,
		Body:  []byte("call-body"),
	})
	response := nextMessage(t, conn)
	if response.Type != packet.Rsp || response.Route != 10 || response.Seq != 1 ||
		string(response.Body) != "node-response" {
		t.Fatalf("Call响应错误: %+v body=%q", response, response.Body)
	}
	callRequest, callStrategy := nodeClient.lastCall()
	if callRequest.GetUid() != "user-1" ||
		callRequest.GetRoute() != 110 ||
		callRequest.GetGateServiceName() != "gate" ||
		callRequest.GetGateInstanceId() != "gate-1" ||
		string(callRequest.GetBody()) != "call-body" {
		t.Fatalf("Node Call请求错误: %+v", callRequest)
	}
	if callStrategy.Mode != lx.ModeBalance ||
		callStrategy.Name != "player" ||
		callStrategy.BalancePolicy != lx.BalanceWeightedRoundRobin {
		t.Fatalf("Call路由策略错误: %+v", callStrategy)
	}

	conn.send(&packet.Message{
		Type:  packet.Req,
		Route: 11,
		Seq:   0,
		Body:  []byte("tell-body"),
	})
	tellRequest, tellStrategy := nodeClient.lastTell()
	if tellRequest.GetUid() != "user-1" || string(tellRequest.GetBody()) != "tell-body" {
		t.Fatalf("Node Tell请求错误: %+v", tellRequest)
	}
	if tellStrategy.Mode != lx.ModeSelect ||
		tellStrategy.Name != "player" ||
		tellStrategy.Key != "user-1" {
		t.Fatalf("Tell路由策略错误: %+v", tellStrategy)
	}
	select {
	case message := <-conn.writes:
		t.Fatalf("Tell不应该写客户端响应: %+v", message)
	default:
	}

	if _, err := g.Push(context.Background(), &pbGate.PushRequest{
		Uid:             "user-1",
		Route:           20,
		Body:            []byte("push"),
		NodeServiceName: "player",
		NodeInstanceId:  "player-1",
	}); err != nil {
		t.Fatal(err)
	}
	push := nextMessage(t, conn)
	if push.Type != packet.Push || push.Route != 20 || string(push.Body) != "push" {
		t.Fatalf("Push错误: %+v body=%q", push, push.Body)
	}

	if _, err := g.Kick(context.Background(), &pbGate.KickRequest{
		Uid:             "user-1",
		NodeServiceName: "player",
		NodeInstanceId:  "player-1",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		t.Fatal("Kick没有关闭连接")
	}
	if got := locatorProvider.locate("user-1"); got != "" {
		t.Fatalf("Session结束后GateLocator仍然存在: %q", got)
	}
	select {
	case err := <-errorChannel:
		t.Fatalf("Gate处理出现错误: %v", err)
	default:
	}

	if serviceRegistry.registered != "gate-1" {
		t.Fatalf("Registry没有注册Gate: %q", serviceRegistry.registered)
	}
	if hookSessionStart.Load() != 1 ||
		hookSessionBind.Load() != 1 ||
		hookSessionEnd.Load() != 1 ||
		hookHeartbeat.Load() != 1 ||
		hookReq.Load() != 2 {
		t.Fatalf(
			"上层Hooks调用次数错误: start=%d bind=%d end=%d heartbeat=%d req=%d",
			hookSessionStart.Load(),
			hookSessionBind.Load(),
			hookSessionEnd.Load(),
			hookHeartbeat.Load(),
			hookReq.Load(),
		)
	}
}

func TestGateRouteErrorUsesErrorHandler(t *testing.T) {
	router := NewRouter()
	nodeClient := &testNodeClient{}
	listener := newTestListener()
	conn := newTestConn()
	listener.connections <- conn
	errorChannel := make(chan error, 1)
	rpcServer := grpc.NewServer()
	rpcListener := bufconn.Listen(1 << 20)

	g, err := New(Config{
		Instance:    endpoint.ServiceInstance{ID: "gate-1", ServiceName: "gate"},
		Router:      router,
		NodeClient:  nodeClient,
		Locator:     locator.NewGateLocator(newTestLocatorProvider(), nil),
		Registry:    &testRegistry{},
		RPCServer:   rpcServer,
		RPCListener: rpcListener,
		NetworkOptions: []network.Option{
			network.WithListener(listener),
			network.WithAuther(network.AuthFunc(func([]byte) (string, error) {
				return "user-1", nil
			})),
		},
		ErrorHandler: func(ctx *Context, err error) {
			errorChannel <- err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = g.Shutdown(context.Background())
	})
	<-conn.started
	bindSession(t, conn)

	conn.send(&packet.Message{Type: packet.Req, Route: 999, Seq: 1})
	select {
	case err := <-errorChannel:
		if !errors.Is(err, ErrRouteNotFound) {
			t.Fatalf("错误=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Route错误没有交给ErrorHandler")
	}
}

func TestGateRPCRejectsInvalidNodeSource(t *testing.T) {
	g := &Gate{}
	if _, err := g.Push(context.Background(), &pbGate.PushRequest{
		Uid:   "user-1",
		Route: 1,
	}); err == nil {
		t.Fatal("Push接受了空Node来源")
	}
	if _, err := g.Kick(context.Background(), &pbGate.KickRequest{
		Uid: "user-1",
	}); err == nil {
		t.Fatal("Kick接受了空Node来源")
	}
}

type testNodeClient struct {
	mutex        sync.Mutex
	response     *pbNode.Response
	callRequest  *pbNode.Request
	tellRequest  *pbNode.Request
	callStrategy lx.Strategy
	tellStrategy lx.Strategy
}

func (c *testNodeClient) Call(
	ctx context.Context,
	request *pbNode.Request,
	options ...grpc.CallOption,
) (*pbNode.Response, error) {
	c.mutex.Lock()
	c.callRequest = proto.Clone(request).(*pbNode.Request)
	c.callStrategy = lx.GetStrategy(ctx)
	response := c.response
	c.mutex.Unlock()
	return response, nil
}

func (c *testNodeClient) Tell(
	ctx context.Context,
	request *pbNode.Request,
	options ...grpc.CallOption,
) (*emptypb.Empty, error) {
	c.mutex.Lock()
	c.tellRequest = proto.Clone(request).(*pbNode.Request)
	c.tellStrategy = lx.GetStrategy(ctx)
	c.mutex.Unlock()
	return &emptypb.Empty{}, nil
}

func (c *testNodeClient) lastCall() (*pbNode.Request, lx.Strategy) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.callRequest, c.callStrategy
}

func (c *testNodeClient) lastTell() (*pbNode.Request, lx.Strategy) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.tellRequest, c.tellStrategy
}

type testLocatorProvider struct {
	mutex    sync.Mutex
	bindings map[string]string
}

func newTestLocatorProvider() *testLocatorProvider {
	return &testLocatorProvider{bindings: make(map[string]string)}
}

func (l *testLocatorProvider) Bind(
	ctx context.Context,
	name string,
	uid string,
	instanceID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mutex.Lock()
	l.bindings[name+":"+uid] = instanceID
	l.mutex.Unlock()
	return nil
}

func (l *testLocatorProvider) Unbind(
	ctx context.Context,
	name string,
	uid string,
	instanceID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mutex.Lock()
	key := name + ":" + uid
	if l.bindings[key] == instanceID {
		delete(l.bindings, key)
	}
	l.mutex.Unlock()
	return nil
}

func (l *testLocatorProvider) Locate(
	ctx context.Context,
	name string,
	uid string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.bindings[name+":"+uid], nil
}

func (l *testLocatorProvider) Close() {}

func (l *testLocatorProvider) locate(uid string) string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.bindings[locator.GateName+":"+uid]
}

type testRegistry struct {
	mutex        sync.Mutex
	registered   string
	deregistered string
}

func (r *testRegistry) Register(instance endpoint.ServiceInstance) error {
	r.mutex.Lock()
	r.registered = instance.ID
	r.mutex.Unlock()
	return nil
}

func (r *testRegistry) Deregister(instanceID string) error {
	r.mutex.Lock()
	r.deregistered = instanceID
	r.mutex.Unlock()
	return nil
}

func (r *testRegistry) UpdateMetaData(string, map[string]string) error {
	return nil
}

func (r *testRegistry) DeleteMetaData(string, []string) error {
	return nil
}

func (r *testRegistry) Close() {}

type testListener struct {
	connections chan transport.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

func newTestListener() *testListener {
	return &testListener{
		connections: make(chan transport.Conn, 2),
		closed:      make(chan struct{}),
	}
}

func (l *testListener) Accept() (transport.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.closed:
		return nil, transport.ErrClosed
	}
}

func (l *testListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *testListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

type testConn struct {
	mutex     sync.Mutex
	handler   transport.Handler
	started   chan struct{}
	writes    chan *packet.Message
	closed    chan struct{}
	closeOnce sync.Once
}

func newTestConn() *testConn {
	return &testConn{
		started: make(chan struct{}),
		writes:  make(chan *packet.Message, 16),
		closed:  make(chan struct{}),
	}
}

func (c *testConn) Start(handler transport.Handler) error {
	c.mutex.Lock()
	c.handler = handler
	c.mutex.Unlock()
	close(c.started)
	return nil
}

func (c *testConn) Write(message *packet.Message) error {
	select {
	case <-c.closed:
		return transport.ErrClosed
	default:
	}
	copyMessage := *message
	copyMessage.Body = append([]byte(nil), message.Body...)
	c.writes <- &copyMessage
	return nil
}

func (c *testConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mutex.Lock()
		handler := c.handler
		c.mutex.Unlock()
		if handler != nil {
			handler.HandleClose(c)
		}
	})
	return nil
}

func (c *testConn) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *testConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *testConn) Type() transport.Type {
	return transport.TypeTCP
}

func (c *testConn) Secure() bool {
	return true
}

func (c *testConn) send(message *packet.Message) {
	c.mutex.Lock()
	handler := c.handler
	c.mutex.Unlock()
	handler.HandleMessage(c, message)
}

func bindSession(t *testing.T, conn *testConn) {
	t.Helper()
	body, err := proto.Marshal(&protocol.BindRequest{Token: []byte("token")})
	if err != nil {
		t.Fatal(err)
	}
	conn.send(&packet.Message{Type: packet.BindReq, Body: body})
	response := nextMessage(t, conn)
	if response.Type != packet.BindRsp {
		t.Fatalf("Bind响应类型=%d", response.Type)
	}
}

func nextMessage(t *testing.T, conn *testConn) *packet.Message {
	t.Helper()
	select {
	case message := <-conn.writes:
		return message
	case <-time.After(time.Second):
		t.Fatal("没有收到网络消息")
		return nil
	}
}
