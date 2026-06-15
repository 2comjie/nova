package kcp

type options struct {
	addr  string
	block interface{} // kcp-go BlockCrypt，nil 表示不加密
}

type Option func(*options)

func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

func defaultOption() *options {
	return &options{
		addr: ":8000",
	}
}
