package lx

import "context"

type ctxKey struct{}

type BalancePolicy string

const (
	ModeDirect  = "direct"
	ModeBalance = "balance"
	ModeNode    = "node"
	ModeSelect  = "select"
	ModeActor   = "actor"
)

const (
	BalanceRoundRobin         BalancePolicy = "round_robin"
	BalanceWeightedRoundRobin BalancePolicy = "weighted_round_robin"
)

type Strategy struct {
	Mode          string
	Service       string
	Binding       string
	Key           string
	Addr          string
	BalancePolicy BalancePolicy
}

func WithStrategy(ctx context.Context, s Strategy) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

func WithDirect(ctx context.Context, addr string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeDirect, Addr: addr})
}

func WithBalance(ctx context.Context, serviceName string, policies ...BalancePolicy) context.Context {
	policy := BalanceWeightedRoundRobin
	if len(policies) > 0 {
		policy = policies[0]
	}
	return WithStrategy(ctx, Strategy{
		Mode:          ModeBalance,
		Service:       serviceName,
		BalancePolicy: policy,
	})
}

func WithNode(ctx context.Context, nodeKey string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeNode, Key: nodeKey})
}

func WithActor(ctx context.Context, serviceName string, actorKey string) context.Context {
	return WithStrategy(ctx, Strategy{Mode: ModeActor, Service: serviceName, Key: actorKey})
}

func WithSelect(
	ctx context.Context,
	serviceName string,
	binding string,
	key string,
) context.Context {
	return WithStrategy(ctx, Strategy{
		Mode:    ModeSelect,
		Service: serviceName,
		Binding: binding,
		Key:     key,
	})
}

func GetStrategy(ctx context.Context) Strategy {
	// 默认是 负载均衡
	strategy := ctx.Value(ctxKey{})
	if strategy == nil {
		return Strategy{Mode: ModeBalance, BalancePolicy: BalanceWeightedRoundRobin}
	}
	s := strategy.(Strategy)
	if s.Mode == ModeBalance && s.BalancePolicy == "" {
		s.BalancePolicy = BalanceWeightedRoundRobin
	}
	return s
}
