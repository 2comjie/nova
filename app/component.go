package app

import "context"

type Component interface {
	Name() string
	Start() error
	Shutdown(context.Context) error
}
