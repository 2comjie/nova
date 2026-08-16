package network

import (
	"time"

	"github.com/2comjie/nova/network/transport"
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

func WithListener(listener transport.Listener) Option {
	return func(options *options) {
		if listener != nil {
			options.listeners = append(options.listeners, listener)
		}
	}
}

func WithDialer(dialer transport.Dialer) Option {
	return func(options *options) {
		options.dialer = dialer
	}
}

func WithAuther(auther Auther) Option {
	return func(options *options) {
		options.auther = auther
	}
}

func WithZipper(zipper Zipper) Option {
	return func(options *options) {
		options.zipper = zipper
	}
}

func WithCryptor(cryptor Cryptor) Option {
	return func(options *options) {
		options.cryptor = cryptor
	}
}

func WithHooks(hooks Hooks) Option {
	return func(options *options) {
		options.hooks = hooks
	}
}

func WithBindTimeout(timeout time.Duration) Option {
	return func(options *options) {
		if timeout > 0 {
			options.bindTimeout = timeout
		}
	}
}

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

func WithMaxBody(size int) Option {
	return func(options *options) {
		if size > 0 {
			options.maxBody = size
		}
	}
}

func WithMaxPending(count int) Option {
	return func(options *options) {
		if count > 0 {
			options.maxPending = count
		}
	}
}
