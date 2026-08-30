package actorDef

import "fmt"

type Type int32
type Key string
type Pid struct {
	Type Type `json:"type"`
	Key  Key  `json:"key"`
}

func (p Pid) String() string {
	return fmt.Sprintf("%d:%s", p.Type, p.Key)
}
