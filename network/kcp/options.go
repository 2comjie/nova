package kcp

import kcp "github.com/xtaci/kcp-go"

type Option func(*options)

type options struct {
	block        kcp.BlockCrypt // 数据块加密，nil 表示不加密
	dataShards   int            // 前向纠错数据分片数
	parityShards int            // 前向纠错冗余分片数
}

func WithBlockCrypt(block kcp.BlockCrypt) Option {
	return func(o *options) {
		o.block = block
	}
}

func WithFEC(dataShards, parityShards int) Option {
	return func(o *options) {
		o.dataShards = dataShards
		o.parityShards = parityShards
	}
}

func defaultOptions() *options {
	return &options{
		block:        nil,
		dataShards:   0,
		parityShards: 0,
	}
}
