package ws

type Option func(*options)

type options struct {
	path     string // WebSocket 升级路径，默认 "/"
	certFile string // TLS 证书文件
	keyFile  string // TLS 秘钥文件
}

func WithPath(path string) Option {
	return func(o *options) {
		o.path = path
	}
}

func WithCertFile(certFile string) Option {
	return func(o *options) {
		o.certFile = certFile
	}
}

func WithKeyFile(keyFile string) Option {
	return func(o *options) {
		o.keyFile = keyFile
	}
}

func defaultOptions() *options {
	return &options{
		path: "/",
	}
}
