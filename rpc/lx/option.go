package lx

import "context"

type ctxKey struct{}

const (
	ModeDirect  = "direct"
	ModeBalance = "balance"
	ModeNode    = "node"
	ModeSelect  = "select"
)

type Strategy struct {
	Mode string
	Name string
	Key  string
	Addr string
}

func WithStrategy(ctx context.Context, s Strategy) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

func WithDirect(ctx context.Context, addr string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeDirect, Addr: addr})
}

func WithBalance(ctx context.Context, serviceName string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeBalance, Name: serviceName})
}

func WithNode(ctx context.Context, nodeKey string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeNode, Key: nodeKey})
}

func WithSelect(ctx context.Context, name, key string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeSelect, Name: name, Key: key})
}

func GetStrategy(ctx context.Context) Strategy {
	// 默认是 负载均衡
	strategy := ctx.Value(ctxKey{})
	if strategy == nil {
		return Strategy{Mode: ModeBalance}
	}
	return strategy.(Strategy)
}
