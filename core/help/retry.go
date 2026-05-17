package help

import (
	"context"
	"time"
)

func Retry(ctx context.Context, attempts int, delay time.Duration, fn func() bool) bool {
	for i := 0; i < attempts; i++ {
		if fn() {
			return true
		}
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(delay):
			}
		}
	}
	return false
}
