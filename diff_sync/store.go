package diff_sync

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/2comjie/nova/logx"
	"golang.org/x/sys/unix"
)

// 0  - 4   Record 总长度
// 4  - 8   CRC32
// 8  - 16  TypeId
// 16 - 24  DataId
// 24 - 32  Version
// 32 - 36  Payload 长度
// 36 - ... Proto Payload

const (
	aofName        = "diff_sync.aof"
	aofRewriteName = "diff_sync.aof.tmp"
	aofFileSize    = 256 * 1024 * 1024
	aofHeaderSize  = 36
)

type record struct {
	TypeId  uint64
	DataId  uint64
	Version uint64
	Payload []byte
}

type Store struct {
	dir     string
	aof     *aof
	rewrite *aof

	writeCh         chan record
	rewriteCh       chan func() error
	rewriteFinished chan bool
	wait            sync.WaitGroup

	flushFailCnt int
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(filepath.Join(dir, aofRewriteName)); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	logFile, err := openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		return nil, err
	}
	store := &Store{
		dir:             dir,
		aof:             logFile,
		writeCh:         make(chan record, 4096),
		rewriteCh:       make(chan func() error),
		rewriteFinished: make(chan bool, 1),
	}
	store.wait.Add(1)
	go store.write()
	return store, nil
}

func (s *Store) write() {
	defer s.wait.Done()

	tk := time.NewTicker(time.Second)
	defer tk.Stop()

	finishRewrite := func(success bool) {
		if !success {
			_ = s.rewrite.Close()
			_ = os.Remove(s.rewrite.path)
			s.rewrite = nil
			return
		}

		// 把 rewrite 刷盘
		if err := s.rewrite.Sync(); err != nil {
			logx.Errorf("diff_sync: 重写 AOF 刷盘失败: %+v", err)
			_ = s.rewrite.Close()
			_ = os.Remove(s.rewrite.path)
			s.rewrite = nil
			return
		}

		// 重写完成之后 直接替换老的aof文件
		if err := os.Rename(s.rewrite.path, s.aof.path); err != nil {
			logx.Errorf("diff_sync: 替换 AOF 文件失败: %v", err)
			_ = s.rewrite.Close()
			_ = os.Remove(s.rewrite.path)
			s.rewrite = nil
			return
		}

		old := s.aof
		s.rewrite.path = old.path
		s.aof = s.rewrite
		s.rewrite = nil
		if err := old.Close(); err != nil {
			logx.Errorf("diff_sync: 关闭旧 AOF 文件失败: %v", err)
		}
	}

	for {
		select {
		case v, ok := <-s.writeCh:
			if !ok {
				// 如果正在重写aof 需要等待写入db的协程完成后才能退出
				if s.rewrite != nil {
					finishRewrite(<-s.rewriteFinished)
				}
				if err := s.aof.Close(); err != nil {
					logx.Errorf("diff_sync: 关闭 AOF 文件错误: %v", err)
				}
				return
			}

			// 写入 aof 文件
			if err := s.aof.Append(v.TypeId, v.DataId, v.Version, v.Payload); err != nil {
				logx.Errorf("diff_sync: 写入 AOF 数据错误: %v", err)
				panic(err)
			}
			// 如果正在重写 采用双写策略 要写入老的aof文件 防止在重写的过程中 进程崩溃丢失数据
			if s.rewrite != nil {
				if err := s.rewrite.Append(v.TypeId, v.DataId, v.Version, v.Payload); err != nil {
					logx.Errorf("diff_sync: 写入 AOF Rewrite 数据错误: %v", err)
					panic(err)
				}
			}
		case <-tk.C:
			err := s.aof.Sync()
			if err != nil {
				s.flushFailCnt++
				logx.Errorf("diff_sync: AOF 刷盘连续失败 %d 次: %v", s.flushFailCnt, err)
				if s.flushFailCnt >= 4 {
					panic(err)
				}
			} else {
				s.flushFailCnt = 0
			}

		case flush := <-s.rewriteCh:
			if s.rewrite != nil {
				continue // 正在重写
			}

			tempPath := filepath.Join(s.dir, aofRewriteName)
			if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
				logx.Errorf("diff_sync: 删除临时 AOF 文件失败: %v", err)
				continue
			}
			value, err := openAOF(tempPath, aofFileSize)
			if err != nil {
				logx.Errorf("diff_sync: 创建临时 AOF 文件失败: %v", err)
				continue
			}
			s.rewrite = value

			go func() {
				if err := flush(); err != nil {
					logx.Errorf("diff_sync: 写入 Db 失败: %v", err)
					s.rewriteFinished <- false
					return
				}
				s.rewriteFinished <- true
			}()

		case success := <-s.rewriteFinished:
			finishRewrite(success)
		}
	}
}

func (s *Store) Shutdown() {
	close(s.writeCh)
	s.wait.Wait()
}

func (s *Store) append(typeId, dataId, version uint64, payload []byte) {
	s.writeCh <- record{
		TypeId:  typeId,
		DataId:  dataId,
		Version: version,
		Payload: payload,
	}
}

func (s *Store) rewriteAOF(flush func() error) {
	s.rewriteCh <- flush
}

type aof struct {
	path   string
	size   int
	file   *os.File
	data   []byte
	offset int
}

func openAOF(path string, size int) (*aof, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	if info.Size() == 0 {
		if err := file.Truncate(int64(size)); err != nil {
			_ = file.Close()
			return nil, err
		}
	} else if info.Size() != int64(size) {
		_ = file.Close()
		return nil, errors.New("diff_sync: AOF 文件大小错误")
	}

	data, err := unix.Mmap(int(file.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	value := &aof{
		path: path,
		size: size,
		file: file,
		data: data,
	}
	for value.offset+aofHeaderSize <= len(value.data) {
		recordSize := int(binary.LittleEndian.Uint32(value.data[value.offset : value.offset+4]))
		if recordSize == 0 {
			break
		}
		if recordSize < aofHeaderSize || value.offset+recordSize > len(value.data) {
			break
		}

		payloadSize := int(binary.LittleEndian.Uint32(value.data[value.offset+32 : value.offset+36]))
		if aofHeaderSize+payloadSize != recordSize {
			break
		}

		checksum := binary.LittleEndian.Uint32(value.data[value.offset+4 : value.offset+8])
		if checksum != crc32.ChecksumIEEE(value.data[value.offset+8:value.offset+recordSize]) {
			break
		}

		value.offset += recordSize
	}

	if value.offset+4 <= len(value.data) {
		clear(value.data[value.offset : value.offset+4])
	}

	return value, nil
}

func (a *aof) Append(typeId, dataId, version uint64, payload []byte) error {
	size := aofHeaderSize + len(payload)
	if size > a.size || a.offset+size > len(a.data) {
		return errors.New("diff_sync: AOF 文件已满")
	}

	recordData := make([]byte, size)
	binary.LittleEndian.PutUint32(recordData[0:4], uint32(size))
	binary.LittleEndian.PutUint64(recordData[8:16], typeId)
	binary.LittleEndian.PutUint64(recordData[16:24], dataId)
	binary.LittleEndian.PutUint64(recordData[24:32], version)
	binary.LittleEndian.PutUint32(recordData[32:36], uint32(len(payload)))
	copy(recordData[aofHeaderSize:], payload)
	binary.LittleEndian.PutUint32(recordData[4:8], crc32.ChecksumIEEE(recordData[8:]))

	copy(a.data[a.offset:], recordData)
	a.offset += size
	if a.offset+4 <= len(a.data) {
		clear(a.data[a.offset : a.offset+4])
	}
	return nil
}

func (a *aof) Replay(apply func(record) error) error {
	for offset := 0; offset < a.offset; {
		size := int(binary.LittleEndian.Uint32(a.data[offset : offset+4]))
		data := make([]byte, size)
		copy(data, a.data[offset:offset+size])

		if err := apply(record{
			TypeId:  binary.LittleEndian.Uint64(data[8:16]),
			DataId:  binary.LittleEndian.Uint64(data[16:24]),
			Version: binary.LittleEndian.Uint64(data[24:32]),
			Payload: data[aofHeaderSize:],
		}); err != nil {
			return err
		}
		offset += size
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
