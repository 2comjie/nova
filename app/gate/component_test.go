package gate

import (
	"errors"
	"testing"

	"github.com/2comjie/nova/app"
)

func TestAddComponentBeforeStart(t *testing.T) {
	gateApp := &Gate{}
	component := &app.CommonComponent{MName: "actor"}
	if err := gateApp.AddComponent(component); err != nil {
		t.Fatal(err)
	}
	if len(gateApp.components) != 1 || gateApp.components[0] != component {
		t.Fatalf("components=%v", gateApp.components)
	}

	gateApp.started.Store(true)
	if err := gateApp.AddComponent(component); !errors.Is(err, ErrStarted) {
		t.Fatalf("error=%v", err)
	}
}
