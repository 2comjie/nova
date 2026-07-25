package network

import (
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/network/transport"
)

// Session 表示一个已接受的连接。ID 只在当前 Server 内自增，UID 在 Bind 成功前为空。
type Session struct {
	ID   uint64
	Conn transport.Conn

	acceptedAt  time.Time
	uid         atomic.Pointer[string]
	boundAt     atomic.Int64
	heartbeatAt atomic.Int64
}

// UID 返回认证得到的用户标识，未 Bind 时返回空字符串。
func (s *Session) UID() string {
	if s == nil {
		return ""
	}
	value := s.uid.Load()
	if value == nil {
		return ""
	}
	return *value
}

// IsBound 表示 Session 是否已经完成 Bind。
func (s *Session) IsBound() bool {
	return s != nil && s.uid.Load() != nil
}

// BoundAt 返回 Bind 成功时间，未 Bind 时返回零值。
func (s *Session) BoundAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	value := s.boundAt.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}
