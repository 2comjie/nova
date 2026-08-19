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
func (s *SnapManager[T]) Commit() bool {
	if !s.state.IsDirty() {
		return false
	}

	writer := NewWriter(nil)
	s.state.WriteDiff(writer)
	s.diffMap[s.version] = writer.Data()
	s.version++
	s.state.ClearDirty()

	if s.version <= s.diffCount {
		return true
	}
	oldestBaseVersion := s.version - s.diffCount
	for baseVersion := range s.diffMap {
		if baseVersion < oldestBaseVersion {
			delete(s.diffMap, baseVersion)
		}
	}
	return true
}

// BuildFull缓存当前版本的Proto全量。生成全量不会改变版本号。
func (s *SnapManager[T]) BuildFull() {
	data, err := proto.Marshal(s.state.GetRawValue())
	if err != nil {
		panic(err)
	}
	s.fullVersion = s.version
	s.fullData = data
}

// Get返回客户端追赶到当前版本所需的数据。
// fullData不为空时先覆盖全量，再按顺序应用diffs；fullData为空时只应用diffs。
// 返回的字节切片由SnapManager持有，调用方只能读取。
func (s *SnapManager[T]) Get(clientVersion uint64) (fullVersion uint64, fullData []byte, diffs []Delta) {
	if clientVersion == s.version {
		return 0, nil, nil
	}
	if clientVersion < s.version {
		if diffs, ok := s.getDiffs(clientVersion); ok {
			return 0, nil, diffs
		}
	}

	if s.fullData != nil {
		if diffs, ok := s.getDiffs(s.fullVersion); ok {
			return s.fullVersion, s.fullData, diffs
		}
	}

	s.BuildFull()
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
