package endpoint

type Status int32

const (
	Work Status = 0
	Hang Status = 1
	Shut Status = 2
)

type ServiceInstance struct {
	ID          string                `json:"id"`           // 服务的ID 必须唯一
	MetaData    map[string]string     `json:"meta_data"`    // 服务的元数据
	Weight      int                   `json:"weight"`       // 负载均衡权重
	Status      Status                `json:"status"`       // 服务状态
	RpcServices map[string]RpcService `json:"rpc_services"` // name -> service
	Routes      map[int32]Router      `json:"routes"`       // route -> router
}

type Router struct {
	Route    int32  `json:"route"`     // 带状态路由
	StateKey string `json:"state_key"` // 根据这个 state key 去 服务注册中心 查找对应的 状态在哪个服务上
}

type RpcService struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}
