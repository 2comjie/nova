package snap

import "google.golang.org/protobuf/proto"

type Result[Message proto.Message] struct {
	BaseVersion uint64
	Version     uint64
	Full        Message
	Updates     []Message
}

type Manager[Client comparable, Message proto.Message] struct {
	version       uint64
	oldestVersion uint64
	updates       []Message
	clients       map[Client]uint64
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
		clients:       make(map[Client]uint64),
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

func (m *Manager[Client, Message]) Bind(id Client, version uint64) {
	m.clients[id] = version
}

func (m *Manager[Client, Message]) Unbind(id Client) {
	delete(m.clients, id)
}

func (m *Manager[Client, Message]) Pull(id Client) Result[Message] {
	baseVersion := m.clients[id]
	if baseVersion > m.version || baseVersion < m.oldestVersion {
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
	return result
}

func (m *Manager[Client, Message]) ClientVersion(id Client) (uint64, bool) {
	version, exists := m.clients[id]
	return version, exists
}

func (m *Manager[Client, Message]) Ack(id Client, version uint64) bool {
	if version > m.version {
		return false
	}
	if version > m.clients[id] {
		m.clients[id] = version
	}
	return true
}
