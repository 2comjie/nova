package network

import (
	"time"

	"github.com/2comjie/wali/packet"
)

type (
	OnStart      func()
	BeforeStop   func()
	OnStop       func()
	OnConnect    func(conn Conn)
	OnDisconnect func(conn Conn)
	OnHeartbeat  func(conn Conn)
	OnMessage    func(conn Conn, message packet.Message)
)

type Options struct {
	OnStart      OnStart
	BeforeStop   BeforeStop
	OnStop       OnStop
	OnConnect    OnConnect
	OnDisconnect OnDisconnect
	OnHeartbeat  OnHeartbeat
	OnMessage    OnMessage

	HeartbeatInterval      time.Duration // 心跳周期
	HeartbeatCheckInterval time.Duration // 心跳检查周期
	Packer                 packet.Packer
	MaxConn                int64
	WriteChSize            int
}
type Option func(*Options)

func WithOnStart(fn OnStart) Option {
	return func(o *Options) { o.OnStart = fn }
}

func WithBeforeStop(fn BeforeStop) Option {
	return func(o *Options) { o.BeforeStop = fn }
}

func WithOnStop(fn OnStop) Option {
	return func(o *Options) { o.OnStop = fn }
}

func WithOnConnect(fn OnConnect) Option {
	return func(o *Options) { o.OnConnect = fn }
}

func WithOnDisconnect(fn OnDisconnect) Option {
	return func(o *Options) { o.OnDisconnect = fn }
}

func WithOnHeartbeat(fn OnHeartbeat) Option {
	return func(o *Options) { o.OnHeartbeat = fn }
}

func WithOnMessage(fn OnMessage) Option {
	return func(o *Options) { o.OnMessage = fn }
}

func WithHeartbeatInterval(d time.Duration) Option {
	return func(o *Options) { o.HeartbeatInterval = d }
}

func WithHeartbeatCheckInterval(d time.Duration) Option {
	return func(o *Options) { o.HeartbeatCheckInterval = d }
}

func WithPacker(p packet.Packer) Option {
	return func(o *Options) { o.Packer = p }
}

func WithMaxConn(n int64) Option {
	return func(o *Options) { o.MaxConn = n }
}

func WithWriteChSize(n int) Option {
	return func(o *Options) { o.WriteChSize = n }
}

func DefaultOption() *Options {
	return &Options{
		OnStart:                nil,
		OnStop:                 nil,
		OnConnect:              nil,
		OnDisconnect:           nil,
		OnHeartbeat:            nil,
		OnMessage:              nil,
		HeartbeatInterval:      time.Duration(20) * time.Second,
		Packer:                 packet.NewPacker(),
		MaxConn:                4000,
		WriteChSize:            1000,
		HeartbeatCheckInterval: time.Duration(30) * time.Second,
	}
}
