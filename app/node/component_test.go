package node

import (
	"errors"
	"testing"

	"github.com/2comjie/nova/app"
)

func TestAddComponentBeforeStart(t *testing.T) {
	nodeApp := &Node{App: app.New()}
	component := &app.CommonComponent{}
	if err := nodeApp.AddComponent(component); err != nil {
		t.Fatal(err)
	}
	if got, ok := nodeApp.GetComponent[*app.CommonComponent](); !ok || got != component {
		t.Fatalf("component=%v exists=%v", got, ok)
	}

	nodeApp.started.Store(true)
	if err := nodeApp.AddComponent(component); !errors.Is(err, ErrStarted) {
		t.Fatalf("error=%v", err)
	}
}
