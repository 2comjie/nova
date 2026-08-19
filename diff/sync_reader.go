package diff

import "encoding/binary"

type SyncReader struct {
	hasFull     bool
	baseVersion uint64
	version     uint64
	fullData    []byte
	diffData    []byte
	nextVersion uint64
	err         error
}

func NewSyncReader(data []byte) (*SyncReader, error) {
	if len(data) == 0 || data[0]&^SyncFlagFull != 0 {
		return nil, ErrInvalidData
	}

	hasFull := data[0]&SyncFlagFull != 0
	header := NewValueReader(data[1:])
	baseVersion := header.Uint64()
	version := header.Uint64()
	var fullData []byte
	if hasFull {
		fullData = header.Bytes()
	}
	diffData := header.Remaining()
	if header.Err() != nil || version < baseVersion {
		return nil, ErrInvalidData
	}

	return &SyncReader{
		hasFull:     hasFull,
		baseVersion: baseVersion,
		version:     version,
		fullData:    fullData,
		diffData:    diffData,
		nextVersion: baseVersion,
	}, nil
}

func (r *SyncReader) HasFull() bool {
	return r.hasFull
}

func (r *SyncReader) BaseVersion() uint64 {
	return r.baseVersion
}

func (r *SyncReader) Version() uint64 {
	return r.version
}

func (r *SyncReader) FullData() []byte {
	return r.fullData
}

func (r *SyncReader) NextDiff() (Delta, bool, error) {
	if r.err != nil {
		return Delta{}, false, r.err
	}
	if r.nextVersion == r.version {
		if len(r.diffData) != 0 {
			r.err = ErrInvalidData
			return Delta{}, false, r.err
		}
		return Delta{}, false, nil
	}

	length, size := binary.Uvarint(r.diffData)
	if size <= 0 {
		r.err = ErrInvalidData
		return Delta{}, false, r.err
	}
	r.diffData = r.diffData[size:]
	if length > uint64(len(r.diffData)) {
		r.err = ErrInvalidData
		return Delta{}, false, r.err
	}

	baseVersion := r.nextVersion
	r.nextVersion++
	delta := Delta{
		BaseVersion: baseVersion,
		Version:     r.nextVersion,
		Data:        r.diffData[:length],
	}
	r.diffData = r.diffData[length:]
	return delta, true, nil
}
