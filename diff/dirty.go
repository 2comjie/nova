package diff

func MarkDirty(word *uint64, mask uint64) bool {
	if *word&mask != 0 {
		return false // 已经是脏的
	}
	*word |= mask // 标记字段脏
	return true
}

func HasDirty(word uint64, mask uint64) bool {
	return word&mask != 0
}

func AnyDirty(bits []uint64) bool {
	for _, word := range bits {
		if word != 0 {
			return true
		}
	}
	return false
}

func ClearDirty(bits []uint64) {
	clear(bits)
}
