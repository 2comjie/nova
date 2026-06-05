package buffer

import (
	"math/bits"
	"sync"

	"github.com/2comjie/wali/core/bytes"
)

var globalBytesPool = NewBytesPool(2 * bytes.MB)

type BytesPool struct {
	pools []*sync.Pool
}

func NewBytesPool(maxSize int) *BytesPool {
	if maxSize < 1 {
		maxSize = 1
	}
	grade := bits.Len(uint(maxSize))
	b := &BytesPool{pools: make([]*sync.Pool, grade)}
	for i := 0; i < grade; i++ {
		size := 1 << i
		b.pools[i] = &sync.Pool{
			New: func() any {
				return make([]byte, size)
			},
		}
	}
	return b
}

func (b *BytesPool) Allocate(size int) []byte {
	if size <= 0 {
		return make([]byte, 0)
	}

	index := b.getIndex(size)
	// 超过最大池化大小，直接创建新切片
	if index >= len(b.pools) {
		return make([]byte, size)
	}

	// 从池获取
	buf := b.pools[index].Get().([]byte)
	// 重置长度为申请大小（容量不变）
	return buf[:size]
}

func (b *BytesPool) Release(buf []byte) {
	if buf == nil || cap(buf) == 0 {
		return
	}

	index := b.getIndex(cap(buf))
	if index >= len(b.pools) {
		return
	}

	clear(buf[:cap(buf)])
	b.pools[index].Put(buf[:cap(buf)])
}

// getIndex 计算 2 的指数索引
func (b *BytesPool) getIndex(n int) int {
	if n <= 1 {
		return 0
	}
	// bytes.Len 会返回最小的位数，刚好对应 2^index >= n
	return bits.Len(uint(n - 1))
}

func AllocateBytes(size int) []byte {
	return globalBytesPool.Allocate(size)
}

func ReleaseBytes(buf []byte) {
	globalBytesPool.Release(buf)
}
