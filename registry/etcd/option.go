package etcd

import "time"

type option struct {
	prefix string
	ttl    time.Duration
}

type Option func(*option)

func WithPrefix(prefix string) Option {
	return func(o *option) {
		o.prefix = prefix
	}
}

func WithTTL(ttl time.Duration) Option {
	return func(o *option) {
		o.ttl = ttl
	}
}

func defaultOption() *option {
	return &option{
		prefix: "/registry",
		ttl:    time.Second * 10,
	}
}
