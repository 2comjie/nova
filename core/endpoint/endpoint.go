package endpoint

import (
	"net"

	"github.com/spf13/cast"
)

type Status int32

const (
	Working  Status = 0
	Hung     Status = 1
	Shutdown Status = 2
)

type ServiceInstance struct {
	ID          string `json:"id"`           // 服务的ID 必须唯一
	ServiceName string `json:"service_name"` // 服务名

	MetaData map[string]string `json:"meta_data"` // 服务的元数据
	Weight   int               `json:"weight"`    // 负载均衡权重
	RpcHost  string            `json:"rpc_host"`
	RpcPort  int               `json:"rpc_port"`
	Status   Status            `json:"status"` // 实例状态 处于 Hung Shutdown 的实例 不会被负载均衡调用到
}

func (s ServiceInstance) RpcTarget() string {
	return net.JoinHostPort(s.RpcHost, cast.ToString(s.RpcPort))
}
