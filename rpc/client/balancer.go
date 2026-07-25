package client

import (
	"context"
	"sync"

	"github.com/2comjie/wali/core/endpoint"
)

// Balancer 从当前服务的可用实例中选择一个实例。
// instances 只在本次调用期间有效，实现方不能修改或长期持有。
type Balancer interface {
	Pick(ctx context.Context, serviceName string, instances []endpoint.ServiceInstance) (endpoint.ServiceInstance, error)
}

type roundRobinBalancer struct {
	mu   sync.Mutex
	next map[string]uint64
}

func newRoundRobinBalancer() *roundRobinBalancer {
	return &roundRobinBalancer{next: make(map[string]uint64)}
}

func (b *roundRobinBalancer) Pick(ctx context.Context, serviceName string, instances []endpoint.ServiceInstance) (endpoint.ServiceInstance, error) {
	if err := ctx.Err(); err != nil {
		return endpoint.ServiceInstance{}, err
	}
	if len(instances) == 0 {
		return endpoint.ServiceInstance{}, ErrNoAnyService
	}

	b.mu.Lock()
	seq := b.next[serviceName]
	b.next[serviceName] = seq + 1
	b.mu.Unlock()
	return instances[seq%uint64(len(instances))], nil
}

type weightedRoundRobinBalancer struct {
	mu      sync.Mutex
	current map[string]map[string]int
}

func newWeightedRoundRobinBalancer() *weightedRoundRobinBalancer {
	return &weightedRoundRobinBalancer{
		current: make(map[string]map[string]int),
	}
}

func (b *weightedRoundRobinBalancer) Pick(ctx context.Context, serviceName string, instances []endpoint.ServiceInstance) (endpoint.ServiceInstance, error) {
	if err := ctx.Err(); err != nil {
		return endpoint.ServiceInstance{}, err
	}
	if len(instances) == 0 {
		return endpoint.ServiceInstance{}, ErrNoAnyService
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	current := b.current[serviceName]
	if current == nil {
		current = make(map[string]int)
		b.current[serviceName] = current
	}

	active := make(map[string]struct{}, len(instances))
	selected := 0
	totalWeight := 0
	for i := range instances {
		instance := instances[i]
		active[instance.ID] = struct{}{}

		weight := instance.Weight
		if weight <= 0 {
			weight = 1
		}
		current[instance.ID] += weight
		totalWeight += weight
		if current[instance.ID] > current[instances[selected].ID] {
			selected = i
		}
	}

	for instanceID := range current {
		if _, ok := active[instanceID]; !ok {
			delete(current, instanceID)
		}
	}
	current[instances[selected].ID] -= totalWeight
	return instances[selected], nil
}
