package diff

import "errors"

var ErrCircularReference = errors.New("diff: 不支持循环引用")

const (
	linkStateVisiting uint8 = 1
	linkStateLinked   uint8 = 2
)

type LinkSession struct {
	states map[*DirtyObject]uint8
}

func (s *LinkSession) Enter(object *DirtyObject, fieldCount uint32, parent Parent, linkId uint32) (bool, error) {
	if s.states == nil {
		s.states = make(map[*DirtyObject]uint8)
	}

	switch s.states[object] {
	case linkStateVisiting:
		return false, ErrCircularReference
	case linkStateLinked:
		if parent != nil {
			object.AddParent(parent, linkId)
		}
		return false, nil
	}

	object.Init(fieldCount)
	if parent != nil {
		object.AddParent(parent, linkId)
	}
	s.states[object] = linkStateVisiting
	return true, nil
}

func (s *LinkSession) Leave(object *DirtyObject) {
	s.states[object] = linkStateLinked
}
