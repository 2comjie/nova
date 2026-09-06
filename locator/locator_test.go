package locator

import (
	"context"
	"testing"
)

type bindingProvider struct {
	Locator
	value string
}

func (p *bindingProvider) Bind(_ context.Context, _, _, value string) (string, error) {
	previous := p.value
	p.value = value
	return previous, nil
}

func (p *bindingProvider) Locate(context.Context, string, string) (string, error) {
	return p.value, nil
}

func TestGateBindDoesNotDecodeUnusedPreviousValue(t *testing.T) {
	provider := &bindingProvider{value: "invalid old binding"}
	locator := NewGateLocator(provider)
	binding := GateBinding{InstanceId: "gate-1", SessionId: 42}
	if err := locator.Bind(context.Background(), 1, binding); err != nil {
		t.Fatal(err)
	}
	current, err := locator.LocateBinding(context.Background(), 1)
	if err != nil || current != binding {
		t.Fatalf("binding=%v err=%v", current, err)
	}
}

func TestLocateBindingRejectsInvalidStoredValue(t *testing.T) {
	for _, value := range []string{"invalid", `{}`, `{"instance_id":"gate-1"}`} {
		locator := NewGateLocator(&bindingProvider{value: value})
		if _, err := locator.LocateBinding(context.Background(), 1); err == nil {
			t.Fatalf("accepted invalid binding %q", value)
		}
	}
}
