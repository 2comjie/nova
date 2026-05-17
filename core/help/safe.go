package help

import "go.uber.org/zap"

func SafeRun(f func()) {
	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorf("panic: %v", r)
		}
	}()
	f()
}

func SafeGo(f func()) {
	go SafeRun(f)
}
