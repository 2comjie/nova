package diff

type PathKeyType uint8

const (
	PathField PathKeyType = 0
	PathMap   PathKeyType = 1
)

type PathNode struct {
	KeyType    PathKeyType
	FieldIndex uint32
	MapKey     any
}

type Path []PathNode

type Operation uint8

const (
	FieldSet     Operation = 1
	FieldClear   Operation = 2
	MapSet       Operation = 3
	MapDelete    Operation = 4
	MapClear     Operation = 5
	SliceReplace Operation = 6
)

type patch struct {
	Path      Path
	Operation Operation
	Value     any
}

type Writer struct {
	patches []patch
}

func NewWriter() *Writer {
	return &Writer{}
}

func SetMap[K comparable, V any](values *map[K]V, key K, value V) {
	if *values == nil {
		*values = make(map[K]V)
	}
	(*values)[key] = value
}

func (w *Writer) writePatch(patch patch) {
	patch.Path = append(Path(nil), patch.Path...)
	w.mergeOverwrite(patch)
}

func (w *Writer) Len() int {
	return len(w.patches)
}

func (w *Writer) Reset() {
	clear(w.patches)
	w.patches = w.patches[:0]
}

func (w *Writer) mergeOverwrite(patch patch) {
	for _, current := range w.patches {
		if (current.Operation == FieldSet || current.Operation == MapSet || current.Operation == SliceReplace) &&
			len(current.Path) < len(patch.Path) && pathWithin(current.Path, patch.Path) {
			return
		}
	}

	writeIndex := 0
	for _, current := range w.patches {
		if pathWithin(patch.Path, current.Path) {
			continue
		}
		w.patches[writeIndex] = current
		writeIndex++
	}

	clear(w.patches[writeIndex:])
	w.patches = w.patches[:writeIndex]
	w.patches = append(w.patches, patch)
}

func pathWithin(parent Path, child Path) bool {
	if len(parent) > len(child) {
		return false
	}

	for index, parentNode := range parent {
		childNode := child[index]
		if parentNode.FieldIndex != childNode.FieldIndex {
			return false
		}

		if index == len(parent)-1 && parentNode.KeyType == PathField {
			return true
		}
		if !samePathNode(parentNode, childNode) {
			return false
		}
	}
	return true
}

func samePathNode(left PathNode, right PathNode) bool {
	if left.FieldIndex != right.FieldIndex || left.KeyType != right.KeyType {
		return false
	}
	if left.KeyType == PathMap {
		return left.MapKey == right.MapKey
	}
	return true
}

func (w *Writer) Commit[Update any](update Update, write func(Update, Path, Operation, any)) Update {
	for _, patch := range w.patches {
		write(update, patch.Path, patch.Operation, patch.Value)
	}
	w.Reset()
	return update
}
