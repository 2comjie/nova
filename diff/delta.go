package diff

import (
	"fmt"
	"strings"
)

type DeltaRoot interface {
	FormatDelta(data []byte) (string, error)
}

type Delta[Root DeltaRoot] []byte

func (d Delta[Root]) Bytes() []byte {
	return d
}

func (d Delta[Root]) Empty() bool {
	return IsEmptyDelta(d)
}

func (d Delta[Root]) String() string {
	var root Root
	value, err := root.FormatDelta(d)
	if err != nil {
		return "invalid delta: " + err.Error()
	}
	return value
}

func IsEmptyDelta(data []byte) bool {
	return len(data) == 1 && data[0] == 0
}

type DebugPatch struct {
	Path      string
	Operation Operation
	Value     any
}

type DebugSnapshot struct {
	Type string
	Size int
}

func (s DebugSnapshot) String() string {
	return fmt.Sprintf("<%s snapshot %d bytes>", s.Type, s.Size)
}

func FormatDebugPatches(root string, patches []DebugPatch) string {
	var builder strings.Builder
	for index, patch := range patches {
		if index != 0 {
			builder.WriteByte('\n')
		}
		fmt.Fprintf(&builder, "%s.%s %s = %v", root, patch.Path, patch.Operation, patch.Value)
	}
	return builder.String()
}

func DebugFieldPath(field string, child string) string {
	if child == "" {
		return field
	}
	return field + "." + child
}

func DebugMapPath(field string, key any, child string) string {
	path := fmt.Sprintf("%s[%v]", field, key)
	if child == "" {
		return path
	}
	return path + "." + child
}

func (o Operation) String() string {
	switch o {
	case PrimitiveSet:
		return "Set"
	case PointerSet:
		return "PointerSet"
	case PointerClear:
		return "PointerClear"
	case MapSet:
		return "MapSet"
	case MapDelete:
		return "MapDelete"
	case MapClear:
		return "MapClear"
	case SliceReplace:
		return "SliceReplace"
	default:
		return fmt.Sprintf("Operation(%d)", o)
	}
}
