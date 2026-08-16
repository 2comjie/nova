package buffer

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/2comjie/nova/core/util"
)

type Bytes struct {
	buf      []byte
	pool     *sync.Pool
	released atomic.Bool
}

func NewBytes(buf []byte) *Bytes {
	b := &Bytes{
		buf: buf,
	}
	b.released.Store(false)
	return b
}

func NewBytesWithCapacity(cap int) *Bytes {
	b := &Bytes{
		buf: make([]byte, cap),
	}
	b.released.Store(false)
	return b
}

func (b *Bytes) Len() int {
	if b == nil {
		return 0
	}
	return len(b.buf)
}

func (b *Bytes) Cap() int {
	if b == nil {
		return 0
	}
	return cap(b.buf)
}

func (b *Bytes) Available() int {
	if b == nil {
		return 0
	}
	return cap(b.buf) - len(b.buf)
}

func (b *Bytes) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.buf
}

func (b *Bytes) Release() {
	if b == nil {
		return
	}
	if b.released.Swap(true) {
		return
	}
	if b.pool != nil {
		b.buf = b.buf[:cap(b.buf)]
		clear(b.buf)
		b.pool.Put(b)
	}
}

func (b *Bytes) WriteTo(w io.Writer) (int64, error) {
	return util.WriteFull(w, b.Bytes())
}
