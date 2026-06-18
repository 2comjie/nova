package client

import (
	"time"

	"github.com/2comjie/wali/packet"
)

type Option func(*options)
type OnHeartBeat func()

type options struct {
	packer            packet.Packer
	heartBeatInterval time.Duration

	onHeartBeat OnHeartBeat
}

func WithPacker(packer packet.Packer) Option {
	return func(o *options) {
		if packer == nil {
			panic("packer is nil")
		}
		o.packer = packer
	}
}

func WithHeartBeatInterval(interval time.Duration) Option {
	return func(o *options) {
		o.heartBeatInterval = interval
	}
}

func WithOnHeartBeat(fn OnHeartBeat) Option {
	return func(o *options) {
		o.onHeartBeat = fn
	}
}

func defaultOptions() *options {
	return &options{
		packer:            packet.NewPacker(),
		heartBeatInterval: time.Duration(30) * time.Second,
	}
}
