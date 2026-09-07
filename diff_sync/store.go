package diff_sync

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"

	"github.com/2comjie/nova/logx"
	"github.com/spf13/cast"
	"golang.org/x/sys/unix"
)

//0  - 4   Record 总长度
//4  - 8   CRC32
//8  - 16  Id
//16 - 24  Version
//24 - 25  Full
//25 - 29  Payload 长度
//29 - ... Proto Payload

const (
	aofSegmentSize = 256 * 1024 * 1024
	aofHeaderSize  = 29
)

type aof struct {
	dir         string
	index       uint64
	segmentSize int

	file   *os.File
	data   []byte
	offset int
}

func openAOF(dir string, segmentSize int) (*aof, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var index uint64

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".aof" {
			continue
		}

		current, err := cast.ToUint64E(strings.TrimSuffix(entry.Name(), ".aof"))
		if err != nil {
			return nil, err
		}

		if current > index {
			index = current
		}
	}

	if index == 0 {
		index = 1
	}

	value := &aof{
		dir:         dir,
		index:       index,
		segmentSize: segmentSize,
	}

	if err := value.open(); err != nil {
		return nil, err
	}

	return value, nil
}

func (a *aof) open() error {
	path := filepath.Join(a.dir, fmt.Sprintf("%06d.aof", a.index))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}

	if info.Size() == 0 {
		if err := file.Truncate(int64(a.segmentSize)); err != nil {
			_ = file.Close()
			return err
		}
	} else if info.Size() != int64(a.segmentSize) {
		_ = file.Close()
		return errors.New("diff_sync: AOF 文件大小错误")
	}

	data, err := unix.Mmap(int(file.Fd()), 0, a.segmentSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		_ = file.Close()
		return err
	}

	a.file = file
	a.data = data
	a.offset = 0
	for a.offset+aofHeaderSize <= len(a.data) {
		size := int(binary.LittleEndian.Uint32(a.data[a.offset : a.offset+4]))
		if size == 0 {
			break
		}
		if size < aofHeaderSize || a.offset+size > len(a.data) {
			break
		}

		payloadSize := int(binary.LittleEndian.Uint32(a.data[a.offset+25 : a.offset+29]))
		if aofHeaderSize+payloadSize != size {
			break
		}

		checksum := binary.LittleEndian.Uint32(a.data[a.offset+4 : a.offset+8])
		if checksum != crc32.ChecksumIEEE(a.data[a.offset+8:a.offset+size]) {
			break
		}

		a.offset += size
	}

	if a.offset+4 <= len(a.data) {
		clear(a.data[a.offset : a.offset+4])
	}

	return nil
}

func (a *aof) Append(id uint64, version uint64, full bool, payload []byte) bool {
	size := aofHeaderSize + len(payload)
	if a.offset+size > len(a.data) {
		return false
	}

	record := make([]byte, size)

	binary.LittleEndian.PutUint32(record[0:4], uint32(size))
	binary.LittleEndian.PutUint64(record[8:16], id)
	binary.LittleEndian.PutUint64(record[16:24], version)

	if full {
		record[24] = 1
	}

	binary.LittleEndian.PutUint32(record[25:29], uint32(len(payload)))
	copy(record[aofHeaderSize:], payload)

	binary.LittleEndian.PutUint32(record[4:8], crc32.ChecksumIEEE(record[8:]))

	copy(a.data[a.offset:], record)
	a.offset += size

	// 标记下一条记录尚不存在，覆盖崩溃前可能残留的旧数据
	if a.offset+4 <= len(a.data) {
		clear(a.data[a.offset : a.offset+4])
	}

	return true
}

func (a *aof) Sync() error {
	err := unix.Msync(a.data, unix.MS_SYNC)
	if err != nil {
		return err
	}
	return nil
}

func (a *aof) Close() {
	saveErr := a.Sync()
	if saveErr != nil {
		logx.Errorf("save aof failed: %v", saveErr)
	}
	unmapErr := unix.Munmap(a.data)
	if unmapErr != nil {
		logx.Errorf("unmap aof failed: %v", unmapErr)
	}
	_ = a.file.Close()
}
