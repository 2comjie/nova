package app

import "context"

type Component interface {
	Name() string
	Start() error
	Shutdown(context.Context) error
}

type CommonComponent struct {
	MName     string
	MStart    func()
	MShutdown func(context.Context) error
}

func (c *CommonComponent) Start() {
	if c.MStart != nil {
		c.MStart()
	}
}
func (c *CommonComponent) Name() string {
	return c.MName
}
func (c *CommonComponent) Shutdown(ctx context.Context) error {
	if c.MShutdown != nil {
		return c.MShutdown(ctx)
	}
	return nil
}
