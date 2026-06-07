package util

func SetBit(b *byte, off int) { *b |= 1 << off }

func ClearBit(b *byte, off int) { *b &^= 1 << off }

func GetBit(b byte, off int) int         { return int(b>>off) & 1 }
func GetBits(b byte, off int, n int) int { return int(b>>off) & ((1 << n) - 1) }
