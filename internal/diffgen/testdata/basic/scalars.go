//go:build diff_fast

package basic

type Scalars struct {
	Enabled bool    `diff:"1"`
	Name    string  `diff:"2"`
	I       int     `diff:"3"`
	I8      int8    `diff:"4"`
	I16     int16   `diff:"5"`
	I32     int32   `diff:"6"`
	I64     int64   `diff:"7"`
	U       uint    `diff:"8"`
	U8      uint8   `diff:"9"`
	U16     uint16  `diff:"10"`
	U32     uint32  `diff:"11"`
	U64     uint64  `diff:"12"`
	F32     float32 `diff:"13"`
	F64     float64 `diff:"14"`
}
