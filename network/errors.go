package network

import "errors"

var (
	ErrAutherRequired  = errors.New("network: 必须提供Auther")
	ErrListenerMissing = errors.New("network: 必须提供至少一个Listener")
	ErrDialerMissing   = errors.New("network: 必须提供Dialer")
	ErrUnauthorized    = errors.New("network: 认证失败")
	ErrAlreadyBound    = errors.New("network: Session已经绑定")
	ErrNotBound        = errors.New("network: Session尚未绑定")
	ErrClosed          = errors.New("network: 已关闭")
	ErrPendingFull     = errors.New("network: 等待响应的请求过多")
	ErrResponseWritten = errors.New("network: Req已经返回响应")
	ErrTellNoResponse  = errors.New("network: Tell不允许返回响应")
)
