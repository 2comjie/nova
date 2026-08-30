//go:build diff_fast

package player

import (
	"github.com/2comjie/nova/examples/diff_app/bag"
	"github.com/2comjie/nova/logx/logdef"
)

type Player struct {
	logdef.ILogger `diff:"-"`

	Uid          uint64           `diff:"1"`
	Level        int32            `diff:"2"`
	Name         string           `diff:"3"`
	Bag          *bag.Bag         `diff:"4"`
	Scores       map[uint64]int32 `diff:"5"`
	RecentLevels []int32          `diff:"6"`
}
