package diff

import (
	"github.com/2comjie/nova/generic"
)

type MapOperation uint8

const (
	MapOperationStore  MapOperation = 1
	MapOperationDelete MapOperation = 2
	MapOperationPatch  MapOperation = 3
)

type mapOperationEntries[K generic.Primitive] []mapOperationEntry[K]

type mapOperationEntry[K generic.Primitive] struct {
	key       K
	operation MapOperation
}

func (e *mapOperationEntries[K]) record(key K, operation MapOperation) {
	for index := range *e {
		entry := &(*e)[index]
		if entry.key != key {
			continue
		}

		// 已经有一个 store 的操作 直接覆盖掉后续的所有 patch 操作
		if entry.operation == MapOperationStore && operation == MapOperationPatch {
			return
		}

		entry.operation = operation
		return
	}

	*e = append(*e, mapOperationEntry[K]{key: key, operation: operation})
}
