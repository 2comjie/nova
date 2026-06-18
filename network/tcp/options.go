package tcp

type Option func(*options)

type options struct {
	certFile string // 证书文件
	keyFile  string // 秘钥文件
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
	return &options{}
}
