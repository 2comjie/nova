package tcp

import (
	"crypto/tls"
	"net"

	"github.com/2comjie/wali/network"
)

type client struct {
	network.BaseClient
	options     *options
	baseOptions *network.Options
}

func NewClient(tcpOpts []Option, baseOpts ...network.Option) network.Client {
	o := defaultOption()
	for _, opt := range tcpOpts {
		opt(o)
	}
	bo := network.DefaultOption()
	for _, opt := range baseOpts {
		opt(bo)
	}
	return &client{options: o, baseOptions: bo}
}

func (c *client) Connect() error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	c.BaseClient.Init(&tcpTransport{conn: conn}, c.baseOptions)
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
