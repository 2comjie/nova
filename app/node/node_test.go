package node

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/2comjie/wali/app"
	"github.com/2comjie/wali/core/endpoint"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestNodeRPC(t *testing.T) {
	provider := newTestLocatorProvider()
	gateLocator := locator.NewGateLocator(provider, nil)
	nodeLocator := locator.NewNodeLocator(provider)
	gateClient := &testGateClient{}
	serviceRegistry := &nodeTestRegistry{}
	var componentMutex sync.Mutex
	var componentCalls []string
	components := []app.Component{
		&nodeTestComponent{name: "matchmaker", mutex: &componentMutex, calls: &componentCalls},
		&nodeTestComponent{name: "map", mutex: &componentMutex, calls: &componentCalls},
	}
	router := NewRouter()
	callHandled := make(chan error, 1)
	tellHandled := make(chan error, 1)

	if err := router.Handle(10, func(ctx *Context) {
		if ctx.App.Instance().ID != "lobby-1" ||
			ctx.Request.UID != "user-1" ||
			ctx.Request.GateServiceName != locator.GateName ||
			ctx.Request.GateInstanceID != "gate-1" ||
			!ctx.NeedReply() ||
			string(ctx.Request.Body) != "call" {
			callHandled <- errors.New("Call Context错误")
			return
		}
		if err := ctx.App.Bind(ctx, "team", "team-1"); err != nil {
			callHandled <- err
			return
		}
		if err := ctx.App.Push(ctx, "user-1", 1001, []byte("push")); err != nil {
			callHandled <- err
			return
		}
		callHandled <- ctx.Reply([]byte("response"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Handle(11, func(ctx *Context) {
		tellHandled <- ctx.Reply([]byte("invalid"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.Handle(12, func(*Context) {
		panic("test")
	}); err != nil {
		t.Fatal(err)
	}

	rpcServer := grpc.NewServer()
	rpcListener := bufconn.Listen(1 << 20)
	nodeApp, err := New(Config{
		Instance: endpoint.ServiceInstance{
			ID:          "lobby-1",
			ServiceName: "lobby",
			RpcHost:     "127.0.0.1",
			RpcPort:     9001,
			Status:      endpoint.Working,
		},
		Router:      router,
		NodeLocator: nodeLocator,
		GateLocator: gateLocator,
		GateClient:  gateClient,
		Registry:    serviceRegistry,
		RPCServer:   rpcServer,
		RPCListener: rpcListener,
		Components:  components,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateLocator.Bind(context.Background(), "user-1", "gate-1"); err != nil {
		t.Fatal(err)
	}
	if err := nodeApp.Start(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(componentCalls, []string{"start:matchmaker", "start:map"}) {
		t.Fatalf("组件启动顺序错误: %v", componentCalls)
	}
	t.Cleanup(func() {
		_ = nodeApp.Shutdown(context.Background())
	})

	client, err := grpc.NewClient(
		"passthrough:///node",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return rpcListener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request := &pbNode.Request{
		Uid:             "user-1",
		Route:           10,
		Body:            []byte("call"),
		GateServiceName: locator.GateName,
		GateInstanceId:  "gate-1",
	}
	response := new(pbNode.Response)
	if err := client.Invoke(ctx, pbNode.Node_Call_FullMethodName, request, response); err != nil {
		t.Fatal(err)
	}
	if err := <-callHandled; err != nil {
		t.Fatal(err)
	}
	if !response.Replied ||
		string(response.Body) != "response" ||
		response.NodeServiceName != "lobby" ||
		response.NodeInstanceId != "lobby-1" {
		t.Fatalf("Call响应错误: %+v", response)
	}
	if gateClient.push == nil ||
		gateClient.push.NodeServiceName != "lobby" ||
		gateClient.push.NodeInstanceId != "lobby-1" ||
		gateClient.pushStrategy.Mode != lx.ModeNode ||
		gateClient.pushStrategy.Key != "gate-1" {
		t.Fatalf("Context App Push错误: request=%+v strategy=%+v", gateClient.push, gateClient.pushStrategy)
	}
	instanceID, err := nodeApp.proxy.Locate(ctx, "team", "team-1")
	if err != nil {
		t.Fatal(err)
	}
	if instanceID != "lobby-1" {
		t.Fatalf("Context App Bind=%q, want lobby-1", instanceID)
	}

	request.Route = 11
	if err := client.Invoke(ctx, pbNode.Node_Tell_FullMethodName, request, &emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
	if err := <-tellHandled; !errors.Is(err, ErrReplyNotAllowed) {
		t.Fatalf("Tell调用Reply返回%v, want %v", err, ErrReplyNotAllowed)
	}

	request.Route = 404
	if err := client.Invoke(ctx, pbNode.Node_Call_FullMethodName, request, response); status.Code(err) != codes.NotFound {
		t.Fatalf("未知route错误=%v, want NotFound", err)
	}

	request.Route = 12
	if err := client.Invoke(ctx, pbNode.Node_Call_FullMethodName, request, response); status.Code(err) != codes.Internal {
		t.Fatalf("Handler panic错误=%v, want Internal", err)
	}

	request.Route = 10
	request.GateServiceName = "fake-gate"
	if err := client.Invoke(ctx, pbNode.Node_Call_FullMethodName, request, response); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("非法Gate来源错误=%v, want InvalidArgument", err)
	}

	if serviceRegistry.registered != "lobby-1" {
		t.Fatalf("Registry注册实例=%q, want lobby-1", serviceRegistry.registered)
	}
	if err := nodeApp.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if serviceRegistry.deregistered != "lobby-1" {
		t.Fatalf("Registry注销实例=%q, want lobby-1", serviceRegistry.deregistered)
	}
	if !reflect.DeepEqual(componentCalls, []string{
		"start:matchmaker",
		"start:map",
		"shutdown:map",
		"shutdown:matchmaker",
	}) {
		t.Fatalf("组件关闭顺序错误: %v", componentCalls)
	}
}

func TestNodeComponentStartRollback(t *testing.T) {
	var mutex sync.Mutex
	var calls []string
	startErr := errors.New("start failed")
	nodeApp := &Node{
		components: []app.Component{
			&nodeTestComponent{name: "first", mutex: &mutex, calls: &calls},
			&nodeTestComponent{name: "second", mutex: &mutex, calls: &calls, startErr: startErr},
			&nodeTestComponent{name: "third", mutex: &mutex, calls: &calls},
		},
	}

	if err := nodeApp.Start(); !errors.Is(err, startErr) {
		t.Fatalf("Start错误=%v, want %v", err, startErr)
	}
	if !reflect.DeepEqual(calls, []string{
		"start:first",
		"start:second",
		"shutdown:first",
	}) {
		t.Fatalf("组件启动失败回滚顺序错误: %v", calls)
	}
}

type nodeTestRegistry struct {
	mutex        sync.Mutex
	registered   string
	deregistered string
}

func (r *nodeTestRegistry) Register(instance endpoint.ServiceInstance) error {
	r.mutex.Lock()
	r.registered = instance.ID
	r.mutex.Unlock()
	return nil
}

func (r *nodeTestRegistry) Deregister(instanceID string) error {
	r.mutex.Lock()
	r.deregistered = instanceID
	r.mutex.Unlock()
	return nil
}

func (r *nodeTestRegistry) UpdateMetaData(string, map[string]string) error {
	return nil
}

func (r *nodeTestRegistry) DeleteMetaData(string, []string) error {
	return nil
}

func (r *nodeTestRegistry) Close() {}

type nodeTestComponent struct {
	name     string
	mutex    *sync.Mutex
	calls    *[]string
	startErr error
}

func (c *nodeTestComponent) Name() string {
	return c.name
}

func (c *nodeTestComponent) Start() error {
	c.mutex.Lock()
	*c.calls = append(*c.calls, "start:"+c.name)
	c.mutex.Unlock()
	return c.startErr
}

func (c *nodeTestComponent) Shutdown(context.Context) error {
	c.mutex.Lock()
	*c.calls = append(*c.calls, "shutdown:"+c.name)
	c.mutex.Unlock()
	return nil
}
