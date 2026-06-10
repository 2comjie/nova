package util

import (
	"os"
	"os/signal"
	"syscall"
)

func WaitUntilSignaled() {
	sigChan := make(chan os.Signal, 10)
	signal.Notify(sigChan, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	select {
	case <-sigChan:
		break
	}
}
