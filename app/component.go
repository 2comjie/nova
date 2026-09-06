package app

import "context"

type Component interface {
	Start() error
	RequestStop()
	Shutdown(context.Context) error
}

type App struct {
	components []Component
	started    int
	stopping   bool
}

func New(components ...Component) *App {
	return &App{components: append([]Component(nil), components...)}
}

func (a *App) AddComponent(component Component) {
	a.components = append(a.components, component)
}

func (a *App) GetComponent[T Component]() (T, bool) {
	for _, component := range a.components {
		if value, ok := component.(T); ok {
			return value, true
		}
	}
	var zero T
	return zero, false
}

func (a *App) Start() error {
	a.stopping = false
	for index, component := range a.components {
		if err := component.Start(); err != nil {
			_ = a.Shutdown(context.Background())
			return err
		}
		a.started = index + 1
	}
	return nil
}

func (a *App) RequestStop() {
	if a.stopping {
		return
	}
	a.stopping = true
	for index := a.started - 1; index >= 0; index-- {
		a.components[index].RequestStop()
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	a.RequestStop()
	var firstErr error
	for index := a.started - 1; index >= 0; index-- {
		if err := a.components[index].Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.started = 0
	return firstErr
}

type CommonComponent struct {
	MStart       func() error
	MRequestStop func()
	MShutdown    func(context.Context) error
}

func (c *CommonComponent) Start() error {
	if c.MStart != nil {
		return c.MStart()
	}
	return nil
}

func (c *CommonComponent) RequestStop() {
	if c.MRequestStop != nil {
		c.MRequestStop()
	}
}

func (c *CommonComponent) Shutdown(ctx context.Context) error {
	if c.MShutdown != nil {
		return c.MShutdown(ctx)
	}
	return nil
}
