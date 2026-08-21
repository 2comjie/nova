package network

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/network/transport"
)

type sessionManager struct {
	mutex            sync.RWMutex
	byConn           map[transport.Conn]*Session
	byID             map[uint64]*Session
	byUID            map[uint64]*Session
	sequence         atomic.Uint64
	auther           Auther
	hooks            Hooks
	bindTimeout      time.Duration
	heartbeatTimeout time.Duration
	stop             chan struct{}
	stopOnce         sync.Once
}

func newSessionManager(options options) *sessionManager {
	return &sessionManager{
		byConn:           make(map[transport.Conn]*Session),
		byID:             make(map[uint64]*Session),
		byUID:            make(map[uint64]*Session),
		auther:           options.auther,
		hooks:            options.hooks,
		bindTimeout:      options.bindTimeout,
		heartbeatTimeout: options.heartbeatTimeout,
		stop:             make(chan struct{}),
	}
}

func (m *sessionManager) Start() {
	help.SafeGo(func() {
		checkInterval := time.Second
		if m.bindTimeout/2 < checkInterval {
			checkInterval = m.bindTimeout / 2
		}
		if m.heartbeatTimeout/2 < checkInterval {
			checkInterval = m.heartbeatTimeout / 2
		}
		if checkInterval < 10*time.Millisecond {
			checkInterval = 10 * time.Millisecond
		}
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case now := <-ticker.C:
				var expired []transport.Conn
				m.mutex.RLock()
				for _, session := range m.byConn {
					if !session.IsBound() {
						if now.Sub(session.acceptedAt) >= m.bindTimeout {
							expired = append(expired, session.Conn)
						}
						continue
					}
					lastHeartbeat := time.Unix(0, session.heartbeatAt.Load())
					if now.Sub(lastHeartbeat) >= m.heartbeatTimeout {
						expired = append(expired, session.Conn)
					}
				}
				m.mutex.RUnlock()
				for _, conn := range expired {
					_ = conn.Close()
				}
			}
		}
	})
}

func (m *sessionManager) Add(conn transport.Conn) *Session {
	now := time.Now()
	session := &Session{
		ID:         m.sequence.Add(1),
		Conn:       conn,
		acceptedAt: now,
	}
	session.heartbeatAt.Store(now.UnixNano())

	m.mutex.Lock()
	m.byConn[conn] = session
	m.byID[session.ID] = session
	m.mutex.Unlock()

	if m.hooks.OnSessionStart != nil {
		help.SafeRun(func() {
			m.hooks.OnSessionStart(session)
		})
	}
	return session
}

func (m *sessionManager) Bind(session *Session, token []byte) error {
	var uid uint64
	var authErr error
	completed := false
	help.SafeRun(func() {
		uid, authErr = m.auther.Auth(token)
		completed = true
	})
	if !completed || authErr != nil || uid == 0 {
		return ErrUnauthorized
	}

	now := time.Now()
	var old *Session
	m.mutex.Lock()
	current, exists := m.byConn[session.Conn]
	if !exists || current != session {
		m.mutex.Unlock()
		return ErrClosed
	}
	if session.IsBound() {
		m.mutex.Unlock()
		return ErrAlreadyBound
	}
	session.uid.Store(uid)
	session.boundAt.Store(now.UnixNano())
	session.heartbeatAt.Store(now.UnixNano())
	old = m.byUID[uid]
	m.byUID[uid] = session
	m.mutex.Unlock()

	if old != nil && old != session {
		_ = old.Conn.Close()
	}
	return nil
}

func (m *sessionManager) Heartbeat(session *Session) {
	m.mutex.RLock()
	if m.byConn[session.Conn] != session {
		m.mutex.RUnlock()
		return
	}
	session.heartbeatAt.Store(time.Now().UnixNano())
	m.mutex.RUnlock()
	if m.hooks.OnHeartbeat != nil {
		help.SafeRun(func() {
			m.hooks.OnHeartbeat(session)
		})
	}
}

func (m *sessionManager) Remove(conn transport.Conn) {
	m.mutex.Lock()
	session, exists := m.byConn[conn]
	if !exists {
		m.mutex.Unlock()
		return
	}
	delete(m.byConn, conn)
	delete(m.byID, session.ID)
	uid := session.UID()
	if uid != 0 && m.byUID[uid] == session {
		delete(m.byUID, uid)
	}
	m.mutex.Unlock()

	if m.hooks.OnSessionEnd != nil {
		help.SafeRun(func() {
			m.hooks.OnSessionEnd(session)
		})
	}
}

func (m *sessionManager) ByConn(conn transport.Conn) *Session {
	m.mutex.RLock()
	session := m.byConn[conn]
	m.mutex.RUnlock()
	return session
}

func (m *sessionManager) ByUID(uid uint64) *Session {
	m.mutex.RLock()
	session := m.byUID[uid]
	m.mutex.RUnlock()
	return session
}

func (m *sessionManager) KickUID(uid uint64) bool {
	session := m.ByUID(uid)
	if session == nil {
		return false
	}
	_ = session.Conn.Close()
	return true
}

func (m *sessionManager) KickSession(id uint64) bool {
	m.mutex.RLock()
	session := m.byID[id]
	m.mutex.RUnlock()
	if session == nil {
		return false
	}
	_ = session.Conn.Close()
	return true
}

func (m *sessionManager) KickUIDSession(uid uint64, id uint64) bool {
	m.mutex.RLock()
	session := m.byID[id]
	m.mutex.RUnlock()
	if session == nil || session.UID() != uid {
		return false
	}
	_ = session.Conn.Close()
	return true
}

func (m *sessionManager) Close() {
	m.stopOnce.Do(func() {
		close(m.stop)
	})

	m.mutex.RLock()
	conns := make([]transport.Conn, 0, len(m.byConn))
	for conn := range m.byConn {
		conns = append(conns, conn)
	}
	m.mutex.RUnlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (m *sessionManager) BoundSessions() []*Session {
	m.mutex.RLock()
	sessions := make([]*Session, 0, len(m.byUID))
	for _, session := range m.byUID {
		sessions = append(sessions, session)
	}
	m.mutex.RUnlock()
	return sessions
}

func (m *sessionManager) ByUIDs(uidList []uint64) []*Session {
	m.mutex.RLock()
	sessions := make([]*Session, 0, len(uidList))
	seen := make(map[*Session]struct{}, len(uidList))

	for _, uid := range uidList {
		session := m.byUID[uid]
		if session == nil {
			continue
		}
		if _, exists := seen[session]; exists {
			continue
		}
		seen[session] = struct{}{}
		sessions = append(sessions, session)
	}

	m.mutex.RUnlock()
	return sessions
}
