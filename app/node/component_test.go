package node

import (
	"errors"
	"testing"

	"github.com/2comjie/wali/app"
)

func TestAddComponentBeforeStart(t *testing.T) {
	nodeApp := &Node{}
	component := &app.CommonComponent{MName: "actor"}
	if err := nodeApp.AddComponent(component); err != nil {
		t.Fatal(err)
	}
	if len(nodeApp.components) != 1 || nodeApp.components[0] != component {
		t.Fatalf("components=%v", nodeApp.components)
	}

	nodeApp.started.Store(true)
	if err := nodeApp.AddComponent(component); !errors.Is(err, ErrStarted) {
		t.Fatalf("error=%v", err)
	}
}
