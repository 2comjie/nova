package diff

import "encoding/binary"

const SyncFlagFull byte = 1

type SyncWriter struct {
	data []byte
}

func NewSyncWriter(data []byte) *SyncWriter {
	return &SyncWriter{data: data}
}

func (w *SyncWriter) Data() []byte {
	return w.data
}

// WriteDiff 写入连续增量 起止版本由第一张和最后一张Delta确定
func (w *SyncWriter) WriteDiff(deltas []Delta) {
	w.data = append(w.data, 0)
	w.data = binary.AppendUvarint(w.data, deltas[0].BaseVersion)
	w.data = binary.AppendUvarint(w.data, deltas[len(deltas)-1].Version)
	for _, delta := range deltas {
		w.data = binary.AppendUvarint(w.data, uint64(len(delta.Data)))
		w.data = append(w.data, delta.Data...)
	}
}

// WriteFull 写入fullVersion全量以及其后的连续增量
func (w *SyncWriter) WriteFull(fullVersion uint64, fullData []byte, deltas []Delta) {
	version := fullVersion
	if len(deltas) != 0 {
		version = deltas[len(deltas)-1].Version
	}
	w.data = append(w.data, SyncFlagFull)
	w.data = binary.AppendUvarint(w.data, fullVersion)
	w.data = binary.AppendUvarint(w.data, version)
	w.data = binary.AppendUvarint(w.data, uint64(len(fullData)))
	w.data = append(w.data, fullData...)
	for _, delta := range deltas {
		w.data = binary.AppendUvarint(w.data, uint64(len(delta.Data)))
		w.data = append(w.data, delta.Data...)
	}
}
