package tcp

import (
	"crypto/tls"
	"net"

	"github.com/2comjie/wali/network"
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

	conn, err := c.dial()
	if err != nil {
		return err
	}
	c.BaseClient.Init(&tcpTransport{conn: conn}, bo)
	return nil
}

func (c *client) dial() (net.Conn, error) {
	if c.options.certFile != "" && c.options.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.options.certFile, c.options.keyFile)
		if err != nil {
			return nil, err
		}
		return tls.Dial("tcp", c.options.addr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
	}
	return net.Dial("tcp", c.options.addr)
}
