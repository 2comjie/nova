package diff

type Operation uint8

const (
	OperationSetVarint  Operation = 1 // 整数
	OperationSetFixed32 Operation = 2 // 32位数值
	OperationSetFixed64 Operation = 3 // 64位数值
	OperationSetBytes   Operation = 4 // Bytes

	OperationClear   Operation = 5 // 清空
	OperationPatch   Operation = 6 // 嵌套Message脏标记
	OperationReplace Operation = 7 // 直接替换了对象

	OperationListAppend Operation = 8  // 添加元素
	OperationListInsert Operation = 9  // 插入元素
	OperationListSet    Operation = 10 // 设置元素
	OperationListDelete Operation = 11 // 删除元素
	OperationListMove   Operation = 12 // 移动元素
	OperationListPatch  Operation = 13 // 拼接元素

	OperationMapPut    Operation = 14 // 添加
	OperationMapDelete Operation = 15 // 删除
	OperationMapPatch  Operation = 16 // 更改
)

type Delta struct {
	BaseVersion uint64
	Version     uint64
	Data        []byte
}
