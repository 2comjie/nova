package diff_sync

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

type record struct {
	Id      uint64
	Version uint64
	Full    bool
	Payload []byte
}

type Store struct {
	aof *aof

	writeCh chan record
	wait    sync.WaitGroup

	flushFailCnt int
}

func NewStore(dir string) (*Store, error) {
	log, err := openAOF(dir, aofSegmentSize)
	if err != nil {
		return nil, err
	}
	store := &Store{
		aof:     log,
		writeCh: make(chan record, 4096),
	}

	store.wait.Add(1)
	go store.write()

	return store, nil
}

func (s *Store) write() {
	defer s.wait.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case value, ok := <-s.writeCh:
			if !ok {
				err := s.aof.Close()
				if err != nil {
					logx.Errorf("diff_sync: 关闭aof文件错误 %+v", err)
				}
				return
			}

			if err := s.aof.Append(value.Id, value.Version, value.Full, value.Payload); err != nil {
				logx.Errorf("diff_sync: 写入aof文件错误 %+v", err)
				// 写入数据都写不进去了 就应该要 直接寄了
				panic(err)
			}
		case <-ticker.C:
			if err := s.aof.Sync(); err != nil {
				logx.Error("diff_sync: aof文件刷盘错误 fail cnt %d %+v", s.flushFailCnt, err)

				s.flushFailCnt++
				if s.flushFailCnt >= 4 {
					// 直接挂掉服务 刷盘刷不进去了
					panic(fmt.Sprintf("diff_sync: aof文件刷盘错误 fail cnt %d %+v", s.flushFailCnt, err))
				}
			} else {
				s.flushFailCnt = 0
			}
		}
	}
}

func (s *Store) Shutdown() {
	close(s.writeCh)
	s.wait.Wait()
}

func (s *Store) append(id uint64, version uint64, full bool, payload []byte) {
	s.writeCh <- record{
		Id:      id,
		Version: version,
		Full:    full,
		Payload: payload,
	}
}

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

func (a *aof) Append(id uint64, version uint64, full bool, payload []byte) error {
	size := aofHeaderSize + len(payload)

	if size > a.segmentSize {
		return errors.New("diff_sync: 单条增量超过 AOF 段大小")
	}

	if a.offset+size > len(a.data) {
		if err := a.rotate(); err != nil {
			return err
		}
	}

	record := make([]byte, size)

	binary.LittleEndian.PutUint32(record[0:4], uint32(size))
	binary.LittleEndian.PutUint64(record[8:16], id)
	binary.LittleEndian.PutUint64(record[16:24], version)

	if full {
		record[24] = 1
	}

	binary.LittleEndian.PutUint32(
		record[25:29],
		uint32(len(payload)),
	)
	copy(record[aofHeaderSize:], payload)

	binary.LittleEndian.PutUint32(record[4:8], crc32.ChecksumIEEE(record[8:]))

	copy(a.data[a.offset:], record)
	a.offset += size

	if a.offset+4 <= len(a.data) {
		clear(a.data[a.offset : a.offset+4])
	}

	return nil
}

func (a *aof) Sync() error {
	return unix.Msync(a.data, unix.MS_SYNC)
}

func (a *aof) Close() error {
	if err := a.Sync(); err != nil {
		return err
	}
	if err := unix.Munmap(a.data); err != nil {
		return err
	}
	return a.file.Close()
}

func (a *aof) rotate() error {
	if err := a.Close(); err != nil {
		return err
	}
	a.index++
	return a.open()
}
