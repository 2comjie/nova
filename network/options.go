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
