package diff

func MarkDirty(bits []uint64, index uint32) bool {
	word := index >> 6                // 右移6相当于除64 计算出在哪个uint64
	mask := uint64(1) << (index & 63) // 计算出在uint64的哪个bit
	if bits[word]&mask != 0 {
		return false // 已经是脏的
	}
	bits[word] |= mask // 标记字段脏
	return true
}

func HasDirty(bits []uint64, index uint32) bool {
	return bits[index>>6]&(uint64(1)<<(index&63)) != 0
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
