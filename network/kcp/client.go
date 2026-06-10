package kcp

import (
	"github.com/2comjie/wali/network"
	kcp "github.com/xtaci/kcp-go"
)

type client struct {
	network.BaseClient
	options *options
}

func NewClient(opts ...Option) network.Client {
	o := defaultOption()
	for _, opt := range opts {
		opt(o)
	}
	return &client{options: o}
}

func (c *client) Connect(baseOpts ...network.Option) error {
	bo := network.DefaultOption()
	for _, opt := range baseOpts {
		opt(bo)
	}

	sess, err := kcp.DialWithOptions(c.options.addr, nil, 0, 0)
	if err != nil {
		return err
	}
	c.BaseClient.Init(&kcpTransport{sess: sess}, bo)
	return nil
}
