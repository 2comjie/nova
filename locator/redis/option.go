package redis

import "time"

type option struct {
	prefix       string
	tick         time.Duration
	ttl          time.Duration
	cacheMaxCost int64
}

type Option func(o *option)

func WithPrefix(prefix string) Option {
	return func(o *option) {
		o.prefix = prefix
	}
}

func WithTick(tick time.Duration) Option {
	return func(o *option) {
		o.tick = tick
	}
}

func WithTTL(ttl time.Duration) Option {
	return func(o *option) {
		o.ttl = ttl
	}
}

func WithCacheMaxCost(n int64) Option {
	return func(o *option) {
		o.cacheMaxCost = n
	}
}

func defaultOption() *option {
	return &option{
		prefix:       "bind",
		tick:         time.Second * 5,
		ttl:          time.Second * 10,
		cacheMaxCost: 10000,
	}
}
