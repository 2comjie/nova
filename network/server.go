package network

type Server interface {
	// Addr 监听地址
	Addr() string
	// Start 启动服务器
	Start(opts ...Option) error
	// Stop 关闭服务器
	Stop() error
	// Protocol 协议
	Protocol() string
	// Conn 按连接ID查找连接
	Conn(id int64) (Conn, bool)
	// ConnByUID 按用户ID查找连接
	ConnByUID(uid string) (Conn, bool)
	// BindUID 绑定连接到用户ID
	BindUID(connID int64, uid string) error
	// UnbindUID 解绑连接上的用户ID
	UnbindUID(connID int64) (string, error)
	// VisitConns 遍历连接，fn 返回 false 时停止遍历
	VisitConns(fn func(Conn) bool)
	// Stat 当前连接数
	Stat() int64
}
