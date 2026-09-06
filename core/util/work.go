package util

import (
	"sync"

	"github.com/2comjie/nova/core/help"
)

func Shard[T, R any](workerCount int, worker func(workerId int) (T, bool), initial R, merge func(result R, value T) R) R {
	if workerCount <= 0 {
		return initial
	}

	type workerResult struct {
		workerId int
		value    T
		ok       bool
	}

	results := make(chan workerResult, workerCount)

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for workerId := 0; workerId < workerCount; workerId++ {
		help.SafeGo(func() {
			defer wg.Done()
			value, ok := worker(workerId)

			results <- workerResult{
				workerId: workerId,
				value:    value,
				ok:       ok,
			}

		})

	}

	wg.Wait()
	close(results)

	workerResults := make([]workerResult, workerCount)
	for result := range results {
		workerResults[result.workerId] = result
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
