package network

import "github.com/2comjie/wali/packet"

type Client interface {
	// Connect 建立连接，addr 由具体实现创建时通过 Option 指定，network.Option 在连接时传入
	Connect(...Option) error
	// Call 同步调用，发 Req 等 Rsp，靠 seq 对齐
	Call(route int32, data []byte) (packet.Message, error)
	// Send 单向发送，不等响应
	Send(route int32, data []byte) error
	// RegisterPushHandler 注册服务端主动推送处理器，按 route 分发
	RegisterPushHandler(route int32, fn func(packet.Message))
	// Close 关闭连接
	Close() error
}
