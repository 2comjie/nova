package buffer_test

import (
	"fmt"
	"testing"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/bytes"
)

func TestAllocate(t *testing.T) {
	pool := buffer.NewBytesPool(16 * bytes.B)
	buf := pool.Allocate(17 * bytes.B)
	fmt.Printf("len %d cap %d value %v", len(buf), cap(buf), buf)
	pool.Release(buf)
}
