package gate

import (
	"errors"
	"testing"

	"github.com/2comjie/nova/app"
)

func TestAddComponentBeforeStart(t *testing.T) {
	gateApp := &Gate{App: app.New()}
	component := &app.CommonComponent{}
	if err := gateApp.AddComponent(component); err != nil {
		t.Fatal(err)
	}
	if got, ok := gateApp.GetComponent[*app.CommonComponent](); !ok || got != component {
		t.Fatalf("component=%v exists=%v", got, ok)
	}

	gateApp.started.Store(true)
	if err := gateApp.AddComponent(component); !errors.Is(err, ErrStarted) {
		t.Fatalf("error=%v", err)
	}
}
