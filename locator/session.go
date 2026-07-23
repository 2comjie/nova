package locator

import "context"

// hash 结构存储
type RouteLocator interface {
	Bind(ctx context.Context, uid string, name string, key string) (string, error) // 绑定 客户端 uid -> 服务名 key
	Unbind(ctx context.Context, uid string, name string) error                     // 解绑
	Locate(ctx context.Context, uid string, name string) (string, error)           // 查询这个 uid 绑定的 服务的 key

	// 1. BindRouteKey
	// 2. UnbindRouteKey
	// 3. 绑定UId
	// BindGate(cid, uid string)
}
