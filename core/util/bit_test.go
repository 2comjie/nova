package util

import "testing"

func TestBit(t *testing.T) {
	// 111
	b := byte(0b110)
	println(b)
	a := GetBits(b, 0, 2)
	println(a)
}
