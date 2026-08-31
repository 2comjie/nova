package help

import "github.com/2comjie/nova/logx"

func SafeRun(f func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("panic: %v", r)
			panicked = true
		}
	}()
	f()
	return false
}

func SafeGo(f func()) {
	go SafeRun(f)
}
