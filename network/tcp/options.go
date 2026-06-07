package tcp

type options struct {
	addr     string
	certFile string
	keyFile  string
}

type Option func(*options)

func WithAddr(addr string) Option {
	return func(o *options) {
		o.addr = addr
	}
}

func WithTLS(certFile string, keyFile string) Option {
	return func(o *options) {
		o.certFile = certFile
		o.keyFile = keyFile
	}
}

func defaultOption() *options {
	return &options{
		addr:     ":8000",
		certFile: "",
		keyFile:  "",
	}
}
