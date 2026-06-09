package ws

import (
	"context"
	"net/http"

	"github.com/2comjie/wali/network"
	"nhooyr.io/websocket"
)

type client struct {
	network.BaseClient
	options     *options
	baseOptions *network.Options
}

func NewClient(wsOpts []Option, baseOpts ...network.Option) network.Client {
	o := defaultOption()
	for _, opt := range wsOpts {
		opt(o)
	}
	bo := network.DefaultOption()
	for _, opt := range baseOpts {
		opt(bo)
	}
	return &client{options: o, baseOptions: bo}
}

func (c *client) Connect() error {
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
	c.BaseClient.Init(newTransport(context.Background(), conn), c.baseOptions)
	return nil
}
