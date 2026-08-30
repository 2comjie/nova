package app

import "context"

type Component interface {
	Start() error
	Shutdown(context.Context) error
}

type App struct {
	components []Component
	started    int
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
	for index, component := range a.components {
		if err := component.Start(); err != nil {
			for rollback := index - 1; rollback >= 0; rollback-- {
				_ = a.components[rollback].Shutdown(context.Background())
			}
			a.started = 0
			return err
		}
		a.started = index + 1
	}
	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
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
	MStart    func() error
	MShutdown func(context.Context) error
}

func (c *CommonComponent) Start() error {
	if c.MStart != nil {
		return c.MStart()
	}
	return nil
}
func (c *CommonComponent) Shutdown(ctx context.Context) error {
	if c.MShutdown != nil {
		return c.MShutdown(ctx)
	}
	return nil
}
