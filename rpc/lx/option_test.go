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
	if strategy.Service != "game" || strategy.BalancePolicy != BalanceRoundRobin {
		t.Fatalf("strategy = %+v", strategy)
	}
}

func TestSelectSeparatesServiceAndBinding(t *testing.T) {
	ctx := WithSelect(context.Background(), "lobby", "team", "team-1")
	strategy := GetStrategy(ctx)
	if strategy.Mode != ModeSelect ||
		strategy.Service != "lobby" ||
		strategy.Binding != "team" ||
		strategy.Key != "team-1" {
		t.Fatalf("strategy = %+v", strategy)
	}
}
