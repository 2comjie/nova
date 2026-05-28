package config

type Option func(*options)

type options struct {
	sources   []Source
	decoder   Decoder
	resolver  func(map[string]any) error
	mergeFunc func(dst, src map[string]any)
}

func defaultOptions() options {
	return options{
		decoder:   defaultDecoder,
		resolver:  defaultResolver,
		mergeFunc: merge,
	}
}

func WithSource(s ...Source) Option {
	return func(o *options) {
		o.sources = s
	}
}

func WithDecoder(d Decoder) Option {
	return func(o *options) {
		o.decoder = d
	}
}

func WithResolveActualTypes(enable bool) Option {
	return func(o *options) {
		o.resolver = func(tree map[string]any) error {
			resolve(tree, enable)
			return nil
		}
	}
}

func WithResolver(r func(map[string]any) error) Option {
	return func(o *options) {
		o.resolver = r
	}
}

func WithMergeFunc(f func(dst, src map[string]any)) Option {
	return func(o *options) {
		o.mergeFunc = f
	}
}

func defaultResolver(tree map[string]any) error {
	resolve(tree, false)
	return nil
}
