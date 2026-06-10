package ws

import (
	"context"
	"net/http"

	"github.com/2comjie/wali/network"
	"nhooyr.io/websocket"
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

	scheme := "ws"
	if c.options.certFile != "" {
		scheme = "wss"
	}
	url := scheme + "://" + c.options.addr + c.options.path

	conn, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPClient: &http.Client{},
	})
	if err != nil {
		return err
	}
	c.BaseClient.Init(newTransport(context.Background(), conn), bo)
	return nil
}
