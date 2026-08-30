//go:build diff_fast

package item

type Item struct {
	ItemId uint64 `diff:"1"`
	Count  int32  `diff:"2"`
}
