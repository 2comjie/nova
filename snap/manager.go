package snap

import "google.golang.org/protobuf/proto"

type Result[Message proto.Message] struct {
	BaseVersion uint64
	Version     uint64
	Full        Message
	Updates     []Message
}

type clientState struct {
	version  uint64
	sent     uint64
	baseline uint64
	ready    bool
	started  bool
}

// Manager 在所属 Actor 内串行使用。Append 和 Pull 的消息在交接后不可修改。
// Client 应标识一次订阅；重新订阅使用新的标识，隔离旧订阅的迟到确认。
type Manager[Client comparable, Message proto.Message] struct {
	version       uint64
	oldestVersion uint64
	updates       []Message
	clients       map[Client]clientState
	snapshot      func() Message
}

func NewManager[Client comparable, Message proto.Message](version uint64, diffCount int, snapshot func() Message) *Manager[Client, Message] {
	if diffCount <= 0 {
		panic("snap: diffCount 必须大于0")
	}
	if snapshot == nil {
		panic("snap: snapshot 不能空")
	}
	return &Manager[Client, Message]{
		version:       version,
		oldestVersion: version,
		updates:       make([]Message, diffCount),
		clients:       make(map[Client]clientState),
		snapshot:      snapshot,
	}
}

func (m *Manager[Client, Message]) Version() uint64 {
	return m.version
}

func (m *Manager[Client, Message]) Append(update Message) uint64 {
	m.version++
	m.updates[m.version%uint64(len(m.updates))] = update
	if m.version-m.oldestVersion > uint64(len(m.updates)) {
		m.oldestVersion = m.version - uint64(len(m.updates))
	}
	return m.version
}

// Bind 建立新订阅，第一次 Pull 必须返回全量。
func (m *Manager[Client, Message]) Bind(id Client) {
	m.clients[id] = clientState{}
}

// Resume 从客户端已有的同一对象、同一版本世代的基线继续同步。
func (m *Manager[Client, Message]) Resume(id Client, version uint64) {
	m.clients[id] = clientState{version: version, ready: true}
}

func (m *Manager[Client, Message]) Unbind(id Client) {
	delete(m.clients, id)
}

func (m *Manager[Client, Message]) Pull(id Client) Result[Message] {
	state, exists := m.clients[id]
	if !exists {
		panic("snap: 客户端尚未绑定")
	}
	baseVersion := state.version
	if !state.ready || baseVersion > m.version || baseVersion < m.oldestVersion {
		if state.ready || !state.started {
			state.baseline = m.version
		}
		state.ready = false
		state.started = true
		state.sent = m.version
		m.clients[id] = state
		return Result[Message]{
			Version: m.version,
			Full:    m.snapshot(),
		}
	}

	result := Result[Message]{
		BaseVersion: baseVersion,
		Version:     m.version,
		Updates:     make([]Message, 0, m.version-baseVersion),
	}
	for version := baseVersion + 1; version <= m.version; version++ {
		result.Updates = append(result.Updates, m.updates[version%uint64(len(m.updates))])
	}
	state.started = true
	state.sent = m.version
	m.clients[id] = state
	return result
}

func (m *Manager[Client, Message]) ClientVersion(id Client) (uint64, bool) {
	state, exists := m.clients[id]
	return state.version, exists && state.ready
}

func (m *Manager[Client, Message]) Ack(id Client, version uint64) bool {
	state, exists := m.clients[id]
	if !exists || !state.started || version > state.sent || version < state.version && state.ready {
		return false
	}
	if !state.ready && version < state.baseline {
		return false
	}
	state.version = version
	state.ready = true
	m.clients[id] = state
	return true
}
