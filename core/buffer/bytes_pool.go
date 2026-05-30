package buffer

import (
	"math/bits"
	"sync"
)

var defaultBytesPool = NewBytesPool(32)

func MallocBytes(cap int) *Bytes {
	return defaultBytesPool.Get(cap)
}

type BytesPool struct {
	pools []*sync.Pool
}

func NewBytesPool(grade int) *BytesPool {
	p := &BytesPool{}
	if grade < 0 {
		return p
	}
	p.pools = make([]*sync.Pool, grade+1)
	for i := range p.pools {
		size := 1 << i
		pool := &sync.Pool{}
		pool.New = func() any { return &Bytes{buf: make([]byte, size), pool: pool} }
		p.pools[i] = pool
	}
	return p
}

func (p *BytesPool) Get(size int) *Bytes {
	pool := p.getPool(size)
	if pool == nil {
		if size < 0 {
			size = 0
		}
		return &Bytes{buf: make([]byte, size)}
	}
	b := pool.Get().(*Bytes)
	b.buf = b.buf[:size]
	b.pool = pool
	b.released.Store(false)
	return b
}

// 获取对象池
func (p *BytesPool) getPool(size int) *sync.Pool {
	if len(p.pools) == 0 {
		return nil
	}

	i := fastCeilLog2(size)
	if i >= len(p.pools) {
		return nil
	}
	return p.pools[i]
}

func fastCeilLog2(n int) int {
	if n <= 1 {
		return 0
	}
	return bits.Len(uint(n - 1))
}
