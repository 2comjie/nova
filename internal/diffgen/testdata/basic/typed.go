//go:build diff_fast

package basic

import clock "time"

type Status int32
type StatusAlias = Status
type IntAlias = int32
type Flag bool
type Clock = clock.Time

const (
	Idle    Status = 0
	Running Status = 1
)

type Typed struct {
	Mode     ModeAlias             `diff:"11"`
	State    Status                `diff:"1"`
	Alias    StatusAlias           `diff:"2"`
	Number   IntAlias              `diff:"3"`
	States   map[Status]Status     `diff:"4"`
	History  []Status              `diff:"5"`
	Flags    map[Flag]Status       `diff:"6"`
	Delay    clock.Duration        `diff:"7"`
	When     Clock                 `diff:"8" json:"when" bson:"when"`
	Times    map[string]clock.Time `diff:"9"`
	Timeline []clock.Time          `diff:"10"`
}
