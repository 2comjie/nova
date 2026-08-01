package config

type Option func(*options)

type options struct {
	sources []Source
}

func WithSource(sources ...Source) Option {
	return func(o *options) {
		o.sources = append(o.sources, sources...)
	}
}
