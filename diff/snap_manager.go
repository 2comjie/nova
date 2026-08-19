package diff

type SnapManager struct {
	diffCount uint64
	version   uint64

	hasFull        bool
	curFullVersion uint64
	curFullBytes   []byte
	diffMap        map[uint64][]byte // base->base+1
}

func NewSnapManager(diffCount uint64) *SnapManager {
	if diffCount == 0 {
		panic("diff: SnapManager diffCount必须大于0")
	}
	return &SnapManager{
		diffCount: diffCount,
		diffMap:   make(map[uint64][]byte, diffCount),
	}
}

func (s *SnapManager) Version() uint64 {
	return s.version
}

func (s *SnapManager) UpdateFull(version uint64, data []byte) {
	if s.hasFull && version < s.curFullVersion {
		return
	}
	// 如果全量 超过了
	if version > s.version {
		clear(s.diffMap)
		s.version = version
	}
	s.hasFull = true
	s.curFullVersion = version
	s.curFullBytes = data
}

func (s *SnapManager) UpdateDiff(baseVersion uint64, version uint64, data []byte) {
	if version != baseVersion+1 {
		panic("diff: Delta必须是单步增量")
	}
	if baseVersion != s.version {
		panic("diff: Delta版本不连续")
	}

	s.diffMap[baseVersion] = data
	s.version = version
	if version <= s.diffCount {
		return
	}

	oldestBaseVersion := version - s.diffCount
	for currentBaseVersion := range s.diffMap {
		if currentBaseVersion < oldestBaseVersion {
			delete(s.diffMap, currentBaseVersion)
		}
	}
}

func (s *SnapManager) Get(clientVersion uint64) (fullVersion uint64, fullBytes []byte, diffs []Delta, ok bool) {
	if clientVersion == s.version {
		return 0, nil, nil, true
	}
	if clientVersion < s.version {
		if diffs, found := s.getDiffs(clientVersion); found {
			return 0, nil, diffs, true
		}
	}
	if !s.hasFull {
		return 0, nil, nil, false
	}

	diffs, found := s.getDiffs(s.curFullVersion)
	if !found {
		return 0, nil, nil, false
	}
	return s.curFullVersion, s.curFullBytes, diffs, true
}

func (s *SnapManager) Reset() {
	s.version = 0
	s.hasFull = false
	s.curFullVersion = 0
	s.curFullBytes = nil
	clear(s.diffMap)
}

func (s *SnapManager) getDiffs(baseVersion uint64) ([]Delta, bool) {
	if baseVersion > s.version {
		return nil, false
	}
	diffs := make([]Delta, 0, len(s.diffMap))
	for version := baseVersion; version < s.version; version++ {
		data, exists := s.diffMap[version]
		if !exists {
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
