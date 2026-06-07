package buffer

import (
	"io"
	"sync/atomic"
)

type NocopyBuffer struct {
	len      int
	num      int
	head     any
	tail     any
	prev     any
	next     any
	delay    atomic.Int32
	released atomic.Bool
}

var _ Buffer = (*NocopyBuffer)(nil)

func NewNocopyBuffer(blocks ...any) *NocopyBuffer {
	buf := &NocopyBuffer{len: -1}
	for _, block := range blocks {
		buf.Mount(block)
	}
	return buf
}

func (b *NocopyBuffer) Len() int {
	if b == nil {
		return 0
	}
	if b.len >= 0 {
		return b.len
	}

	size := 0
	b.Visit(func(node *NocopyNode) bool {
		size += node.Len()
		return true
	})
	b.len = size
	return size
}

func (b *NocopyBuffer) Mount(block any, whence ...Whence) {
	if b == nil || block == nil {
		return
	}

	node := mountNode(block)
	if node == nil {
		return
	}

	if len(whence) > 0 && whence[0] == Head {
		b.addToHead(node)
		return
	}
	b.addToTail(node)
}

func (b *NocopyBuffer) MallocBytes(size int, whence ...Whence) *Bytes {
	block := MallocBytes(size)
	b.Mount(block, whence...)
	return block
}

func (b *NocopyBuffer) MallocWriter(size int, whence ...Whence) *Writer {
	block := MallocWriter(size)
	b.Mount(block, whence...)
	return block
}

func (b *NocopyBuffer) Visit(fn func(node *NocopyNode) bool) bool {
	if b == nil || fn == nil {
		return true
	}

	for node := b.head; node != nil; {
		switch n := node.(type) {
		case *NocopyNode:
			next := n.next
			if !fn(n) {
				return false
			}
			node = next
		case *NocopyBuffer:
			next := n.next
			if !n.Visit(fn) {
				return false
			}
			node = next
		default:
			return false
		}
	}
	return true
}

func (b *NocopyBuffer) VisitBytes(fn func([]byte) bool) bool {
	if fn == nil {
		return true
	}
	return b.Visit(func(node *NocopyNode) bool {
		return fn(node.Bytes())
	})
}

func (b *NocopyBuffer) WriteTo(w io.Writer) (int64, error) {
	if b == nil || w == nil {
		return 0, nil
	}

	var written int64
	var writeErr error
	b.VisitBytes(func(chunk []byte) bool {
		if len(chunk) == 0 {
			return true
		}
		n, err := w.Write(chunk)
		written += int64(n)
		if err != nil {
			writeErr = err
			return false
		}
		if n != len(chunk) {
			writeErr = io.ErrShortWrite
			return false
		}
		return true
	})
	return written, writeErr
}

func (b *NocopyBuffer) Bytes() []byte {
	if b == nil {
		return nil
	}

	switch b.num {
	case 0:
		return nil
	case 1:
		switch h := b.head.(type) {
		case *NocopyNode:
			return h.Bytes()
		case *NocopyBuffer:
			return h.Bytes()
		default:
			return nil
		}
	default:
		out := make([]byte, 0, b.Len())
		b.VisitBytes(func(chunk []byte) bool {
			out = append(out, chunk...)
			return true
		})
		return out
	}
}

func (b *NocopyBuffer) Delay(delay int32) {
	if b == nil {
		return
	}
	b.delay.Store(delay)
}

func (b *NocopyBuffer) Release() {
	if b == nil {
		return
	}
	if !b.releaseDelay() {
		return
	}
	if !b.released.CompareAndSwap(false, true) {
		return
	}

	for node := b.head; node != nil; {
		switch n := node.(type) {
		case *NocopyNode:
			next := n.next
			n.Release()
			node = next
		case *NocopyBuffer:
			next := n.next
			n.Release()
			node = next
		default:
			node = nil
		}
	}
	b.len = -1
	b.num = 0
	b.head = nil
	b.tail = nil
	b.prev = nil
	b.next = nil
}

func (b *NocopyBuffer) releaseDelay() bool {
	for {
		delay := b.delay.Load()
		if delay <= 0 {
			return true
		}
		if b.delay.CompareAndSwap(delay, delay-1) {
			return delay-1 <= 0
		}
	}
}

func (b *NocopyBuffer) addToHead(node any) {
	if node == nil {
		return
	}
	setPrev(node, nil)
	if b.head == nil {
		setNext(node, nil)
		b.head = node
		b.tail = node
	} else {
		setNext(node, b.head)
		setPrev(b.head, node)
		b.head = node
	}
	b.invalidateLen()
	b.num += nodeNum(node)
}

func (b *NocopyBuffer) addToTail(node any) {
	if node == nil {
		return
	}
	setNext(node, nil)
	if b.tail == nil {
		setPrev(node, nil)
		b.head = node
		b.tail = node
	} else {
		setPrev(node, b.tail)
		setNext(b.tail, node)
		b.tail = node
	}
	b.invalidateLen()
	b.num += nodeNum(node)
}

func (b *NocopyBuffer) invalidateLen() {
	b.len = -1
}

func mountNode(block any) any {
	switch v := block.(type) {
	case *NocopyNode:
		return v
	case *NocopyBuffer:
		return v
	case []byte:
		return &NocopyNode{block: v}
	case *Bytes:
		return &NocopyNode{block: v}
	case *Writer:
		return &NocopyNode{block: v}
	default:
		return nil
	}
}

func nodeNum(node any) int {
	switch n := node.(type) {
	case *NocopyNode:
		return 1
	case *NocopyBuffer:
		return n.num
	default:
		return 0
	}
}

func setPrev(node any, prev any) {
	switch n := node.(type) {
	case *NocopyNode:
		n.prev = prev
	case *NocopyBuffer:
		n.prev = prev
	}
}

func setNext(node any, next any) {
	switch n := node.(type) {
	case *NocopyNode:
		n.next = next
	case *NocopyBuffer:
		n.next = next
	}
}
