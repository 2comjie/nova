package kcp

import (
	"github.com/2comjie/wali/network"
	kcp "github.com/xtaci/kcp-go"
)

type client struct {
	network.BaseClient
	options     *options
	baseOptions *network.Options
}

func NewClient(kcpOpts []Option, baseOpts ...network.Option) network.Client {
	o := defaultOption()
	for _, opt := range kcpOpts {
		opt(o)
	}
	bo := network.DefaultOption()
	for _, opt := range baseOpts {
		opt(bo)
	}
	return &client{options: o, baseOptions: bo}
}

func (c *client) Connect() error {
	sess, err := kcp.DialWithOptions(c.options.addr, nil, 0, 0)
	if err != nil {
		return err
	}
	c.BaseClient.Init(&kcpTransport{sess: sess}, c.baseOptions)
	return nil
}
