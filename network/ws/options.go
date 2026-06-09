package ws

type options struct {
	addr     string
	certFile string
	keyFile  string
	path     string // WebSocket 升级路径，默认 "/"
}

type Option func(*options)

func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

func WithTLS(certFile, keyFile string) Option {
	return func(o *options) {
		o.certFile = certFile
		o.keyFile = keyFile
	}
}

func WithPath(path string) Option {
	return func(o *options) { o.path = path }
}

func defaultOption() *options {
	return &options{
		addr: ":8000",
		path: "/",
	}
}
