package diffgen

import "testing"

func TestGenerate(t *testing.T) {
	if err := Generate("../../examples/diff_app/player"); err != nil {
		t.Fatal(err)
	}
}
