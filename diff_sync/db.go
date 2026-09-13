package diff_sync

import "github.com/2comjie/nova/generic"

type Key interface {
	generic.Integer | ~string
}

type DbRecord[K Key] struct {
	DataId       K
	SavedVersion uint64 // 上次确认落库的版本 作为本次写入的条件
	Version      uint64 // 本次数据版本
	Data         []byte // 完整二进制数据
}
