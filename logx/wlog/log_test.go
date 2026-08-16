package wlog

import (
	"runtime"
	"strings"
	"testing"
)

func TestCallerUsesFullPath(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	caller, _ := getCallInfoN(1)
	if !strings.HasPrefix(caller, file+":") {
		t.Fatalf("caller=%s", caller)
	}
}
