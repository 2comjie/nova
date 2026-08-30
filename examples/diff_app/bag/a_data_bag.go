//go:build diff_fast

package bag

import "github.com/2comjie/nova/examples/diff_app/item"

type Bag struct {
	Items map[uint64]*item.Item[int32] `diff:"1"`
	Order []*item.Item[int32]          `diff:"2"`
}
