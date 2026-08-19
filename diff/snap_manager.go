package diff

import "google.golang.org/protobuf/proto"

type SnapState[T proto.Message] interface {
	GetRawValue() T
	IsDirty() bool
	WriteDiff(writer *Writer)
	ClearDirty()
}

type SnapManager[T proto.Message] struct {
	state     SnapState[T]
	diffCount uint64
	version   uint64

	hasFull     bool
	fullVersion uint64
	fullData    []byte
	diffMap     map[uint64][]byte
}

func NewSnapManager[T proto.Message](state SnapState[T], version uint64, diffCount uint64) *SnapManager[T] {
	if diffCount == 0 {
		panic("diff: diffCount必须大于0")
	}
	return &SnapManager[T]{
		state:     state,
		diffCount: diffCount,
		version:   version,
		diffMap:   make(map[uint64][]byte, diffCount),
	}
}

func (s *SnapManager[T]) Version() uint64 {
	return s.version
}

// Commit 把当前脏数据提交为 version -> version+1 的单步增量
func (s *SnapManager[T]) Commit() (Delta, bool) {
	if !s.state.IsDirty() {
		return Delta{}, false
	}

	baseVersion := s.version
	writer := NewWriter(nil)
	s.state.WriteDiff(writer)
	delta := Delta{
		BaseVersion: baseVersion,
		Version:     baseVersion + 1,
		Data:        writer.Data(),
	}
	s.diffMap[baseVersion] = delta.Data
	s.version = delta.Version
	s.state.ClearDirty()

	if s.version <= s.diffCount {
		return delta, true
	}
	oldestBaseVersion := s.version - s.diffCount
	for baseVersion := range s.diffMap {
		if baseVersion < oldestBaseVersion {
			delete(s.diffMap, baseVersion)
		}
	}
	return delta, true
}

func (s *SnapManager[T]) BuildSnapshot() (uint64, []byte) {
	s.buildFull()
	return s.fullVersion, s.fullData
}

func (s *SnapManager[T]) WriteSync(clientVersion uint64, buffer []byte) ([]byte, bool) {
	fullVersion, fullData, deltas := s.get(clientVersion)
	if fullData == nil && len(deltas) == 0 {
		return nil, false
	}

	writer := NewSyncWriter(buffer)
	if fullData != nil {
		writer.WriteFull(fullVersion, fullData, deltas)
	} else {
		writer.WriteDiff(deltas)
	}
	return writer.Data(), true
}

func (s *SnapManager[T]) buildFull() {
	data, err := proto.Marshal(s.state.GetRawValue())
	if err != nil {
		panic(err)
	}
	if data == nil {
		data = []byte{}
	}
	s.hasFull = true
	s.fullVersion = s.version
	s.fullData = data
}

func (s *SnapManager[T]) get(clientVersion uint64) (fullVersion uint64, fullData []byte, diffs []Delta) {
	if clientVersion == s.version {
		return 0, nil, nil
	}
	if clientVersion < s.version {
		if diffs, ok := s.getDiffs(clientVersion); ok {
			return 0, nil, diffs
		}
	}

	if s.hasFull {
		if diffs, ok := s.getDiffs(s.fullVersion); ok {
			return s.fullVersion, s.fullData, diffs
		}
	}

	s.buildFull()
	return s.fullVersion, s.fullData, nil
}

func (s *SnapManager[T]) getDiffs(baseVersion uint64) ([]Delta, bool) {
	if baseVersion > s.version {
		return nil, false
	}
	if baseVersion == s.version {
		return nil, true
	}

	diffs := make([]Delta, 0, s.version-baseVersion)
	for version := baseVersion; version < s.version; version++ {
		data, ok := s.diffMap[version]
		if !ok {
			return nil, false
		}
		diffs = append(diffs, Delta{
			BaseVersion: version,
			Version:     version + 1,
			Data:        data,
		})
	}
	return diffs, true
}
