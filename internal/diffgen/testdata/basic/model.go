//go:build diff_fast

package basic

type Model struct {
	Level    int32             `diff:"1" json:"level"`
	Scores   map[uint64]int32  `diff:"2"`
	Child    *Child            `diff:"3"`
	Children map[uint64]*Child `diff:"4"`
	Order    []*Child          `diff:"5"`
}
