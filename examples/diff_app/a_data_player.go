//go:build diff_fast

package main

type Player struct {
	Uid          uint64           `diff:"1"`
	Level        int32            `diff:"2"`
	Name         string           `diff:"3"`
	Bag          *Bag             `diff:"4"`
	Scores       map[uint64]int32 `diff:"5"`
	RecentLevels []int32          `diff:"6"`
}

type Bag struct {
	Items map[uint64]*Item `diff:"1"`
	Order []*Item          `diff:"2"`
}

type Item struct {
	ItemId uint64 `diff:"1"`
	Count  int32  `diff:"2"`
}
