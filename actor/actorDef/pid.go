package actorDef

import "fmt"

type Type int32
type Key string
type PID struct {
	Type Type `json:"type"`
	Key  Key  `json:"key"`
}

func (p PID) BindingName() string {
	return fmt.Sprintf("actor:%d", p.Type)
}

func (p PID) BindingKey() string {
	return fmt.Sprintf("%v", p.Key)
}
func (p PID) String() string {
	return fmt.Sprintf("%d:%s", p.Type, p.Key)
}
