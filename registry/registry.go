package registry

import (
	"context"

	"github.com/2comjie/wali/core/endpoint"
)

type Registry interface {
	Register(instance endpoint.ServiceInstance) error
	Deregister(instanceID string) error
	UpdateMetaData(instanceId string, meta map[string]string) error // 更新服务元数据
	DeleteMetaData(instanceId string, keys []string) error          // 删除 meta data
	Close()
}
type Discover interface {
	List(ctx context.Context) (map[string]endpoint.ServiceInstance, error)
	Next(ctx context.Context) (map[string]endpoint.ServiceInstance, error)
	Get(ctx context.Context, instanceID string) (endpoint.ServiceInstance, bool, error)
	Close()
}
