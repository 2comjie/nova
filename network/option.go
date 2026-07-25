package network

import (
	"time"

	"github.com/2comjie/wali/network/transport"
)

const (
	defaultBindTimeout      = 5 * time.Second
	defaultHeartbeat        = 30 * time.Second
	defaultHeartbeatTimeout = 90 * time.Second
	defaultMaxBody          = 8 << 20
	defaultMaxToken         = 8 << 10
	defaultMaxPending       = 1024
)

type options struct {
	listeners        []transport.Listener
	dialer           transport.Dialer
	auther           Auther
	zipper           Zipper
	cryptor          Cryptor
	hooks            Hooks
	bindTimeout      time.Duration
	heartbeat        time.Duration
	heartbeatTimeout time.Duration
	maxBody          int
	maxToken         int
	maxPending       int
}

// Option 同时用于 Server 和 Client，未使用的配置会被对应一端忽略。
type Option func(*options)

func defaultOptions() options {
	return options{
		bindTimeout:      defaultBindTimeout,
		heartbeat:        defaultHeartbeat,
		heartbeatTimeout: defaultHeartbeatTimeout,
		maxBody:          defaultMaxBody,
		maxToken:         defaultMaxToken,
		maxPending:       defaultMaxPending,
	}
}

// WithListener 添加 Server 监听器，可重复调用以同时监听 TCP、KCP 和 WebSocket。
func WithListener(listener transport.Listener) Option {
	return func(options *options) {
		if listener != nil {
			options.listeners = append(options.listeners, listener)
		}
	}
}

// WithDialer 设置 Client 使用的拨号器。
func WithDialer(dialer transport.Dialer) Option {
	return func(options *options) {
		options.dialer = dialer
	}
}

// WithAuther 设置 BindReq 的 token 认证实现。
func WithAuther(auther Auther) Option {
	return func(options *options) {
		options.auther = auther
	}
}

// WithZipper 设置业务 Body 压缩实现。
func WithZipper(zipper Zipper) Option {
	return func(options *options) {
		options.zipper = zipper
	}
}

// WithCryptor 设置业务 Body 加密实现。
func WithCryptor(cryptor Cryptor) Option {
	return func(options *options) {
		options.cryptor = cryptor
	}
}

// WithHooks 设置五个业务钩子。
func WithHooks(hooks Hooks) Option {
	return func(options *options) {
		options.hooks = hooks
	}
}

// WithBindTimeout 设置连接建立后等待 BindReq 的最长时间。
func WithBindTimeout(timeout time.Duration) Option {
	return func(options *options) {
		if timeout > 0 {
			options.bindTimeout = timeout
		}
	}
}

// WithHeartbeat 设置客户端心跳周期和服务端 Session 失效时间。
func WithHeartbeat(interval, timeout time.Duration) Option {
	return func(options *options) {
		if interval > 0 {
			options.heartbeat = interval
		}
		if timeout > 0 {
			options.heartbeatTimeout = timeout
		}
	}
}

// WithMaxBody 设置解密、解压后的业务 Body 上限。
func WithMaxBody(size int) Option {
	return func(options *options) {
		if size > 0 {
			options.maxBody = size
		}
	}
}

// WithMaxPending 设置 Client 同时等待的 Call 数量。
func WithMaxPending(count int) Option {
	return func(options *options) {
		if count > 0 {
			options.maxPending = count
		}
	}
}
