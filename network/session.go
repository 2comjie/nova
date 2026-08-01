package network

import (
	"sync/atomic"
	"time"

	"github.com/2comjie/wali/network/transport"
)

type Session struct {
	ID   uint64
	Conn transport.Conn

	acceptedAt  time.Time
	uid         atomic.Pointer[string]
	boundAt     atomic.Int64
	heartbeatAt atomic.Int64
}

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

func (s *Session) IsBound() bool {
	return s != nil && s.uid.Load() != nil
}

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
