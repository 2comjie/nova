package snap

import "github.com/2comjie/nova/diff"

type Result struct {
	BaseVersion uint64
	Version     uint64
	Full        bool
	Snapshot    []byte
	Deltas      [][]byte
}

type Manager struct {
	version       uint64
	oldestVersion uint64
	deltas        [][]byte
	clients       map[uint64]uint64
	snapshot      func() []byte // 每次都会分配 []byte 出来？每次 Pull？
}

func NewManager(version uint64, diffCount int, snapshot func() []byte) *Manager {
	if diffCount <= 0 {
		panic("snap: diffCount必须大于0")
	}
	if snapshot == nil {
		panic("snap: snapshot不能为空")
	}
	return &Manager{
		version:       version,
		oldestVersion: version,
		deltas:        make([][]byte, diffCount),
		clients:       make(map[uint64]uint64),
		snapshot:      snapshot,
	}
}

func (m *Manager) Version() uint64 {
	return m.version
}

func (m *Manager) Append(delta []byte) uint64 {
	if diff.IsEmptyDelta(delta) {
		return m.version
	}
	m.version++
	m.deltas[m.version%uint64(len(m.deltas))] = delta

	if m.version-m.oldestVersion > uint64(len(m.deltas)) {
		m.oldestVersion = m.version - uint64(len(m.deltas))
	}
	return m.version
}

func (m *Manager) Bind(uid uint64, version uint64) {
	m.clients[uid] = version
}

func (m *Manager) Unbind(uid uint64) {
	delete(m.clients, uid)
}

func (m *Manager) ClientVersion(uid uint64) (uint64, bool) {
	version, exists := m.clients[uid]
	return version, exists
}

func (m *Manager) Pull(uid uint64) Result {
	baseVersion := m.clients[uid]
	if baseVersion > m.version || baseVersion < m.oldestVersion {
		return Result{
			Version:  m.version,
			Full:     true,
			Snapshot: m.snapshot(),
		}
	}

	result := Result{
		BaseVersion: baseVersion,
		Version:     m.version,
		Deltas:      make([][]byte, 0, m.version-baseVersion),
	}
	for version := baseVersion + 1; version <= m.version; version++ {
		result.Deltas = append(result.Deltas, m.deltas[version%uint64(len(m.deltas))])
	}
	return result
}

func (m *Manager) Ack(uid uint64, version uint64) bool {
	if version > m.version {
		return false
	}
	if version > m.clients[uid] {
		m.clients[uid] = version
	}
	return true
}
