package network

import (
	"net"

	"github.com/2comjie/wali/core/buffer"
)

const (
	ConnOpened ConnState = iota + 1 // 连接打开
	ConnHanged                      // 连接挂起
	ConnClosed                      // 连接关闭
)

type (
	ConnState int32

	Conn interface {
		// ID 获取连接ID
		ID() int64
		// UID 获取用户ID
		UID() string
		// Attr 属性接口
		Attr() Attr
		// Bind 绑定用户ID
		Bind(uid string)
		// Unbind 解绑用户ID
		Unbind()
		// Send 发送消息（同步）
		Send(msg buffer.Buffer) error
		// Push 发送消息（异步）
		Push(msg buffer.Buffer) error
		// State 获取连接状态
		State() ConnState
		// Close 关闭连接
		Close(reason string, force ...bool) error
		// LocalIP 获取本地IP
		LocalIP() (string, error)
		// LocalAddr 获取本地地址
		LocalAddr() (net.Addr, error)
		// RemoteIP 获取远端IP
		RemoteIP() (string, error)
		// RemoteAddr 获取远端地址
		RemoteAddr() (net.Addr, error)
	}

	Attr interface {
		// Set 设置属性值
		Set(key, value string)
		// Get 获取属性值
		Get(key string) (string, bool)
		// Del 删除属性值
		Del(key string) bool
		// Visit 访问所有的属性值
		Visit(fn func(key, value string) bool)
	}
)
