package deploy

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nodeapp "github.com/2comjie/wali/app/node"
	"github.com/2comjie/wali/config"
	"github.com/2comjie/wali/config/file"
	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/network"
	nettcp "github.com/2comjie/wali/network/transport/tcp"
	"github.com/2comjie/wali/registry"
)

func TestNodeAutoRPCPortAndGracefulShutdown(t *testing.T) {
	configCenter := &testConfig{Config: config.New()}
	serviceRegistry := &testRegistry{}
	discover := newTestDiscover()
	locatorProvider := &testLocator{}
	router := nodeapp.NewRouter()
	if err := router.Handle(10, func(*nodeapp.Context) {}); err != nil {
		t.Fatal(err)
	}

	node, err := Node(
		WithServiceName("lobby"),
		WithInstanceID("lobby-1"),
		WithConfig(configCenter),
		WithRegistry(serviceRegistry),
		WithDiscover(discover),
		WithLocator(locatorProvider),
		WithNodeRouter(router),
		WithRPCListener(newTestNetListener(31001)),
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := node.Instance()
	if instance.RpcHost != "127.0.0.1" || instance.RpcPort <= 0 {
		t.Fatalf("自动RPC地址无效: %+v", instance)
	}

	persisted := make(chan struct{})
	node.AddWait()
	help.SafeGo(func() {
		defer node.DoneWait()
		<-node.Done()
		if configCenter.closed.Load() {
			t.Error("后台任务退出前Config已经关闭")
		}
		close(persisted)
	})

	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	if registered := serviceRegistry.Registered(); !reflect.DeepEqual(registered, instance) {
		t.Fatalf("注册实例=%+v, want %+v", registered, instance)
	}
	if err := node.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-persisted:
	default:
		t.Fatal("Node没有等待后台任务退出")
	}
	if !configCenter.loaded.Load() || !configCenter.closed.Load() {
		t.Fatalf(
			"Config生命周期错误: loaded=%v closed=%v",
			configCenter.loaded.Load(),
			configCenter.closed.Load(),
		)
	}
	if serviceRegistry.deregistered != "lobby-1" || !serviceRegistry.closed.Load() {
		t.Fatalf(
			"Registry生命周期错误: deregistered=%q closed=%v",
			serviceRegistry.deregistered,
			serviceRegistry.closed.Load(),
		)
	}
	if !discover.closed.Load() || !locatorProvider.closed.Load() {
		t.Fatal("Discover或Locator没有关闭")
	}
}

func TestGateLoadsRoutesFromConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "gate.json")
	if err := os.WriteFile(configPath, []byte(`{
		"gate": {
			"routes": [{
				"id": "lobby",
				"routes": [1001],
				"target": {
					"mode": "balance",
					"service": "lobby"
				}
			}]
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configCenter := &testConfig{
		Config: config.New(config.WithSource(file.NewSource(configPath))),
	}
	serviceRegistry := &testRegistry{}
	discover := newTestDiscover()
	locatorProvider := &testLocator{}
	clientListener := nettcp.NewListener(newTestNetListener(32001))

	gate, err := Gate(
		WithServiceName(locator.GateName),
		WithInstanceID("gate-1"),
		WithConfig(configCenter),
		WithRegistry(serviceRegistry),
		WithDiscover(discover),
		WithLocator(locatorProvider),
		WithRPCListener(newTestNetListener(31002)),
		WithNetworkOptions(
			network.WithListener(clientListener),
			network.WithAuther(network.AuthFunc(func([]byte) (string, error) {
				return "user-1", nil
			})),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.Start(); err != nil {
		t.Fatal(err)
	}
	if gate.Instance().RpcPort <= 0 {
		t.Fatal("Gate没有生成RPC端口")
	}
	if err := gate.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !configCenter.closed.Load() ||
		!serviceRegistry.closed.Load() ||
		!discover.closed.Load() ||
		!locatorProvider.closed.Load() {
		t.Fatal("Gate deploy资源没有全部关闭")
	}
}

type testConfig struct {
	config.Config
	loaded atomic.Bool
	closed atomic.Bool
}

func (c *testConfig) Load() error {
	c.loaded.Store(true)
	return c.Config.Load()
}

func (c *testConfig) Close() error {
	c.closed.Store(true)
	return c.Config.Close()
}

type testRegistry struct {
	mutex        sync.Mutex
	registered   endpoint.ServiceInstance
	deregistered string
	closed       atomic.Bool
}

func (r *testRegistry) Register(instance endpoint.ServiceInstance) error {
	r.mutex.Lock()
	r.registered = instance
	r.mutex.Unlock()
	return nil
}

func (r *testRegistry) Deregister(instanceID string) error {
	r.mutex.Lock()
	r.deregistered = instanceID
	r.mutex.Unlock()
	return nil
}

func (r *testRegistry) Registered() endpoint.ServiceInstance {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.registered
}

func (r *testRegistry) UpdateMetaData(string, map[string]string) error {
	return nil
}

func (r *testRegistry) DeleteMetaData(string, []string) error {
	return nil
}

func (r *testRegistry) Close() {
	r.closed.Store(true)
}

type testDiscover struct {
	closed atomic.Bool
}

func newTestDiscover() *testDiscover {
	return &testDiscover{}
}

func (d *testDiscover) List(context.Context) (map[string]endpoint.ServiceInstance, error) {
	return map[string]endpoint.ServiceInstance{}, nil
}

func (d *testDiscover) Next(ctx context.Context) (map[string]endpoint.ServiceInstance, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (d *testDiscover) Get(
	context.Context,
	string,
) (endpoint.ServiceInstance, bool, error) {
	return endpoint.ServiceInstance{}, false, nil
}

func (d *testDiscover) Close() {
	d.closed.Store(true)
}

type testLocator struct {
	mutex  sync.Mutex
	values map[string]string
	closed atomic.Bool
}

type testNetListener struct {
	address net.Addr
	closed  chan struct{}
	once    sync.Once
}

func newTestNetListener(port int) *testNetListener {
	return &testNetListener{
		address: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port},
		closed:  make(chan struct{}),
	}
}

func (l *testNetListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, net.ErrClosed
}

func (l *testNetListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *testNetListener) Addr() net.Addr {
	return l.address
}

func (l *testLocator) Bind(
	_ context.Context,
	name string,
	key string,
	instanceID string,
) error {
	l.mutex.Lock()
	if l.values == nil {
		l.values = make(map[string]string)
	}
	l.values[name+":"+key] = instanceID
	l.mutex.Unlock()
	return nil
}

func (l *testLocator) Unbind(
	_ context.Context,
	name string,
	key string,
	instanceID string,
) error {
	l.mutex.Lock()
	if l.values[name+":"+key] == instanceID {
		delete(l.values, name+":"+key)
	}
	l.mutex.Unlock()
	return nil
}

func (l *testLocator) Locate(
	_ context.Context,
	name string,
	key string,
) (string, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.values[name+":"+key], nil
}

func (l *testLocator) Close() {
	l.closed.Store(true)
}

var (
	_ registry.Registry = (*testRegistry)(nil)
	_ registry.Discover = (*testDiscover)(nil)
	_ locator.Locator   = (*testLocator)(nil)
)

func TestShutdownTimeoutDefaults(t *testing.T) {
	options := defaultOptions()
	if options.shutdownTimeout != defaultShutdownTimeout {
		t.Fatalf(
			"默认Shutdown超时=%s, want %s",
			options.shutdownTimeout,
			defaultShutdownTimeout,
		)
	}
	if options.weight != 1 {
		t.Fatalf("默认权重=%d, want 1", options.weight)
	}
	if options.rpcListen != "127.0.0.1:0" {
		t.Fatalf("默认RPC监听=%q", options.rpcListen)
	}
}

func TestWithShutdownTimeoutIgnoresInvalidValue(t *testing.T) {
	options := defaultOptions()
	WithShutdownTimeout(-time.Second)(&options)
	if options.shutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("非法Shutdown超时覆盖了默认值: %s", options.shutdownTimeout)
	}
}
