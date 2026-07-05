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
	ID          string                `json:"id"`           // 服务的ID 必须唯一
	MetaData    map[string]string     `json:"meta_data"`    // 服务的元数据
	Weight      int                   `json:"weight"`       // 负载均衡权重
	RpcServices map[string]RpcService `json:"rpc_services"` // name -> service
	Routes      map[int32]Router      `json:"routes"`       // route -> router
	RpcHost     string                `json:"rpc_host"`
	RpcPort     int                   `json:"rpc_port"`
	Status      Status                `json:"status"` // 实例状态 处于 Hung Shutdown 的实例 不会被负载均衡调用到
}

func (s ServiceInstance) RpcTarget() string {
	return net.JoinHostPort(s.RpcHost, cast.ToString(s.RpcPort))
}

type Router struct {
	Route      int32  `json:"route"`
	StateGroup string `json:"state_group"` // node节点调用BindRoute(group, nodeId) 可以把这一组路由绑定到这个node节点上
}

type RpcService struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}
