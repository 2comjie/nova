//go:build diff_fast

package basic

type Collections struct {
	Runtime  string             `diff:"-"`
	Signed   map[int8]int16     `diff:"1"`
	Unsigned map[uint16]uint8   `diff:"2"`
	Flags    map[bool]string    `diff:"3"`
	Floats   map[string]float32 `diff:"4"`
	Doubles  []float64          `diff:"5"`
	Counts   []int              `diff:"6"`
	Bytes    []byte             `diff:"7"`
}
