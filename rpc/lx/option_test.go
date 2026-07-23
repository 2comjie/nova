package lx

import (
	"context"
	"testing"
)

func TestBalancePolicy(t *testing.T) {
	if got := GetStrategy(context.Background()).BalancePolicy; got != BalanceWeightedRoundRobin {
		t.Fatalf("default balance policy = %q, want %q", got, BalanceWeightedRoundRobin)
	}

	ctx := WithBalance(context.Background(), "game", BalanceRoundRobin)
	strategy := GetStrategy(ctx)
	if strategy.Name != "game" || strategy.BalancePolicy != BalanceRoundRobin {
		t.Fatalf("strategy = %+v", strategy)
	}
}
