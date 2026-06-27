package lx

import (
	"context"
)

type ctxKey struct{}

type Mode int

const (
	ModeDirect Mode = iota
	ModeNode
	ModeKey
)

type CallConfig struct {
	Mode   Mode
	Target string

	Name string
	Key  string
}

func WithNode(ctx context.Context, nodeID string) context.Context {
	return set(ctx, &CallConfig{Mode: ModeNode, Target: nodeID})
}

func WithDirect(ctx context.Context, addr string) context.Context {
	return set(ctx, &CallConfig{Mode: ModeDirect, Target: addr})
}

func WithSelect(ctx context.Context, name, key string) context.Context {
	return set(ctx, &CallConfig{Mode: ModeKey, Name: name, Key: key})
}

func set(ctx context.Context, cfg *CallConfig) context.Context {
	return context.WithValue(ctx, ctxKey{}, cfg)
}

func FromContext(ctx context.Context) *CallConfig {
	cfg, _ := ctx.Value(ctxKey{}).(*CallConfig)
	return cfg
}
