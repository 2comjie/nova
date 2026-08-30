//go:build diff_fast

package item

type Item[Count ~int32 | ~int64] struct {
	ItemId uint64 `diff:"1"`
	Count  Count  `diff:"2"`
}
