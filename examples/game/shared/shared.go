package shared

import (
	"github.com/2comjie/wali/deploy"
	"github.com/2comjie/wali/locator"
	redisLocator "github.com/2comjie/wali/locator/redis"
	"github.com/2comjie/wali/registry"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	"github.com/redis/go-redis/v9"
)

const (
	RouteChatSend     uint32 = 1001
	RouteChatPush     uint32 = 1002
	RoutePlayerGet    uint32 = 2001
	RoutePlayerAddExp uint32 = 2002
)

type ChatSendRequest struct {
	ToUID string `json:"to_uid"`
	Text  string `json:"text"`
}

type ChatSendResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type ChatPush struct {
	FromUID string `json:"from_uid"`
	Text    string `json:"text"`
}

type PlayerProfile struct {
	UID   string `json:"uid"`
	Level int    `json:"level"`
	Exp   int    `json:"exp"`
	Gold  int    `json:"gold"`
}

type PlayerAddExpRequest struct {
	Exp int `json:"exp"`
}

// Infrastructure 是三个演示服务共用的Redis基础设施。
type Infrastructure struct {
	Redis    redis.UniversalClient
	Registry registry.Registry
	Discover registry.Discover
	Locator  locator.Locator
}

func NewInfrastructure(address string) *Infrastructure {
	redisClient := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{address},
	})
	return &Infrastructure{
		Redis:    redisClient,
		Registry: redisRegistry.NewRegistry(redisClient),
		Discover: redisRegistry.NewDiscover(redisClient),
		Locator:  redisLocator.NewProvider(redisClient),
	}
}

// DeployOptions 返回Gate和Node需要的公共部署参数。
func (i *Infrastructure) DeployOptions() []deploy.Option {
	return []deploy.Option{
		deploy.WithRegistry(i.Registry),
		deploy.WithDiscover(i.Discover),
		deploy.WithLocator(i.Locator),
	}
}
