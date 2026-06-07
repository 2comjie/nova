package buffer

import "io"

type Whence int

const (
	Head Whence = iota
	Tail
)

const (
	b8 = 1 << iota
	b16
	b32
	b64
)

type Buffer interface {
	Len() int
	Bytes() []byte
	Release()
	WriteTo(w io.Writer) (int64, error)
}
