//go:build diff_fast

package basic

type Child struct {
	Count int32 `diff:"1"`
}
