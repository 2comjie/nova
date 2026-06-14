package help

import "github.com/2comjie/wali/logx"

func SafeRun(f func()) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("panic: %v", r)
		}
	}()
	f()
}

func SafeGo(f func()) {
	go SafeRun(f)
}
