package buffer

import "sync"

var defaultWriterPool = NewWriterPool(32)

// MallocWriter 分配一块内存给Writer
func MallocWriter(cap int) *Writer {
	return defaultWriterPool.Get(cap)
}

type WriterPool struct {
	pools []*sync.Pool
}

// NewWriterPool 分级创建写入器池
func NewWriterPool(grade int) *WriterPool {
	p := &WriterPool{}
	if grade < 0 {
		return p
	}
	p.pools = make([]*sync.Pool, grade+1)

	for i := range p.pools {
		size := 1 << i
		pool := &sync.Pool{}
		pool.New = func() any { return &Writer{buf: make([]byte, size), pool: pool} }
		p.pools[i] = pool
	}

	return p
}

// NewWriterPoolWithCapacity 以指定容量创建写入器池
func NewWriterPoolWithCapacity(size int) *WriterPool {
	return NewWriterPool(fastCeilLog2(size))
}

// Get 获取
func (p *WriterPool) Get(size int) *Writer {
	pool := p.getPool(size)

	if pool == nil {
		if size < 0 {
			size = 0
		}
		return NewWriterWithCapacity(size)
	}

	w := pool.Get().(*Writer)
	w.off = 0
	w.pool = pool
	w.released.Store(false)
	return w
}

// Put 放回
func (p *WriterPool) Put(w *Writer) {
	if w == nil {
		return
	}
	w.Release()
}

// 获取对象池
func (p *WriterPool) getPool(size int) *sync.Pool {
	if len(p.pools) == 0 {
		return nil
	}

	i := fastCeilLog2(size)
	if i >= len(p.pools) {
		return nil
	}
	return p.pools[i]
}
