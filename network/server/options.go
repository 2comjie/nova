package server

import (
	"time"

	"github.com/2comjie/wali/packet"
)

type Option func(*options)

type (
	OnConnect    func(conn Conn)
	OnDisconnect func(conn Conn, reason string)
	OnHeartbeat  func(conn Conn)
	OnMessage    func(conn Conn, message packet.Message)
)

type options struct {
	onConnect    OnConnect
	onDisconnect OnDisconnect
	onHeartbeat  OnHeartbeat
	onMessage    OnMessage

	heartBeatInterval time.Duration
	packer            packet.Packer
	maxConn           int
	writChSize        int
}

func WithOnConnect(fn OnConnect) Option {
	return func(o *options) {
		o.onConnect = fn
	}
}
func WithOnDisconnect(fn OnDisconnect) Option {
	return func(o *options) {
		o.onDisconnect = fn
	}
}
func WithOnHeartbeat(fn OnHeartbeat) Option {
	return func(o *options) {
		o.onHeartbeat = fn
	}
}
func WithOnMessage(fn OnMessage) Option {
	return func(o *options) {
		o.onMessage = fn
	}
}
func WithHeartBeatInterval(interval time.Duration) Option {
	return func(o *options) {
		o.heartBeatInterval = interval
	}
}
func WithPacker(packer packet.Packer) Option {
	return func(o *options) {
		if packer == nil {
			panic("packer is nil")
		}
		o.packer = packer
	}
}
func WithMaxConn(maxConn int) Option {
	return func(o *options) {
		if maxConn <= 0 {
			panic("maxConn must be greater than 0")
		}
		o.maxConn = maxConn
	}
}
func WithWriteChSize(size int) Option {
	return func(o *options) {
		if size <= 0 {
			panic("writeChSize must be greater than 0")
		}
		o.writChSize = size
	}
}
func defaultOptions() *options {
	return &options{
		onConnect:         nil,
		onDisconnect:      nil,
		onHeartbeat:       nil,
		onMessage:         nil,
		heartBeatInterval: time.Duration(30) * time.Second,
		packer:            packet.NewPacker(),
		maxConn:           4000,
		writChSize:        1000,
	}
}
