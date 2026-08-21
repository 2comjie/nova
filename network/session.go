package network

import (
	"sync/atomic"
	"time"

	"github.com/2comjie/nova/network/transport"
)

type Session struct {
	ID   uint64
	Conn transport.Conn

	acceptedAt  time.Time
	uid         atomic.Uint64
	boundAt     atomic.Int64
	heartbeatAt atomic.Int64
}

func (s *Session) UID() uint64 {
	if s == nil {
		return 0
	}
	return s.uid.Load()
}

func (s *Session) IsBound() bool {
	return s != nil && s.uid.Load() != 0
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
