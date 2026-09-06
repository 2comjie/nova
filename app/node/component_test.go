package node

import (
	"testing"

	"github.com/2comjie/nova/app"
)

func TestAddComponentBeforeStart(t *testing.T) {
	nodeApp := &Node{App: app.New()}
	component := &app.CommonComponent{}
	nodeApp.AddComponent(component)
	if got, ok := nodeApp.GetComponent[*app.CommonComponent](); !ok || got != component {
		t.Fatalf("component=%v exists=%v", got, ok)
	}
}
