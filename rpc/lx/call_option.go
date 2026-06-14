package lx

import "context"

type ctxKey struct{}

// Mode represents the routing strategy.
type Mode int

const (
	ModeDirect Mode = iota
	ModeNode
	ModeKey
)

// CallConfig holds the routing override for a single call.
type CallConfig struct {
	Mode   Mode
	Target string
}

// WithNode sets the routing strategy to target a specific node.
func WithNode(ctx context.Context, nodeID string) context.Context {
	return set(ctx, &CallConfig{Mode: ModeNode, Target: nodeID})
}

// WithDirect sets the routing strategy to connect to a specific address.
func WithDirect(ctx context.Context, addr string) context.Context {
	return set(ctx, &CallConfig{Mode: ModeDirect, Target: addr})
}

// WithRouteKey sets the routing strategy to use key-based routing.
func WithRouteKey(ctx context.Context, key string) context.Context {
	return set(ctx, &CallConfig{Mode: ModeKey, Target: key})
}

func set(ctx context.Context, cfg *CallConfig) context.Context {
	return context.WithValue(ctx, ctxKey{}, cfg)
}

// FromContext extracts a CallConfig from ctx. Returns nil if none set.
func FromContext(ctx context.Context) *CallConfig {
	cfg, _ := ctx.Value(ctxKey{}).(*CallConfig)
	return cfg
}
