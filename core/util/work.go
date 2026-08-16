package util

import (
	"sync"

	"github.com/2comjie/nova/core/help"
)

func Shard[T, R any](workerCount int, worker func(workerID int) (T, bool), initial R, merge func(result R, value T) R) R {
	if workerCount <= 0 {
		return initial
	}

	type workerResult struct {
		workerID int
		value    T
		ok       bool
	}

	results := make(chan workerResult, workerCount)

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for workerID := 0; workerID < workerCount; workerID++ {
		help.SafeGo(func() {
			defer wg.Done()
			value, ok := worker(workerID)

			results <- workerResult{
				workerID: workerID,
				value:    value,
				ok:       ok,
			}

		})

	}

	wg.Wait()
	close(results)

	workerResults := make([]workerResult, workerCount)
	for result := range results {
		workerResults[result.workerID] = result
	}

	merged := initial
	for _, result := range workerResults {
		if !result.ok {
			continue
		}
		merged = merge(merged, result.value)
	}

	return merged
}
