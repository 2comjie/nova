package diff

type SliceOperation uint8

const (
	SliceOperationAppend SliceOperation = 1
	SliceOperationInsert SliceOperation = 2
	SliceOperationSet    SliceOperation = 3
	SliceOperationDelete SliceOperation = 4
	SliceOperationMove   SliceOperation = 5
)

type sliceOperationEntries[T any] []sliceOperationEntry[T]

type sliceOperationEntry[T any] struct {
	value     T
	index     uint32
	toIndex   uint32
	operation SliceOperation
}

func (e *sliceOperationEntries[T]) record(operation SliceOperation, index uint32, toIndex uint32, value T) {
	*e = append(*e, sliceOperationEntry[T]{
		value:     value,
		index:     index,
		toIndex:   toIndex,
		operation: operation,
	})
}
