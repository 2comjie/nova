package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/2comjie/nova/app/node"
	"github.com/2comjie/nova/core/help"
	"github.com/2comjie/nova/deploy"
	"github.com/2comjie/nova/examples/game/shared"
	"github.com/2comjie/nova/flag"
	"github.com/2comjie/nova/logx"
	"github.com/redis/go-redis/v9"
)

const playerDataKey = "nova:demo:players"

func main() {
	infrastructure := shared.NewInfrastructure(
		flag.String("redis", "127.0.0.1:6379"),
	)
	defer infrastructure.Redis.Close()

	store := &playerStore{
		redis:    infrastructure.Redis,
		profiles: make(map[uint64]shared.PlayerProfile),
	}
	router := node.NewRouter()
	router.Handle(shared.RoutePlayerGet, store.onGet)
	router.Handle(shared.RoutePlayerAddExp, store.onAddExp)

	options := infrastructure.DeployOptions()
	options = append(options,
		deploy.WithServiceName(flag.String("service", "player")),
		deploy.WithInstanceID(flag.String("id", "player-1")),
		deploy.WithNodeRouter(router),
		deploy.WithComponents(store),
	)
	player, err := deploy.Node(options...)
	if err != nil {
		panic(err)
	}

	// 持久化协程由App统一管理，Shutdown会关闭Done并等待DoneWait。
	player.AddWait()
	help.SafeGo(func() {
		defer player.DoneWait()
		store.persistLoop(player.Done())
	})

	logx.Infof("Player Node启动 id=%s rpc=%s", player.Instance().ID, player.Instance().RpcTarget())
	if err := player.Run(); err != nil {
		panic(err)
	}
}

type playerStore struct {
	redis    redis.UniversalClient
	mutex    sync.Mutex
	profiles map[uint64]shared.PlayerProfile
}

func (s *playerStore) Name() string {
	return "player-store"
}

func (s *playerStore) Start() error {
	logx.Infof("玩家数据组件启动")
	return nil
}

func (s *playerStore) Shutdown(context.Context) error {
	logx.Infof("玩家数据组件关闭")
	return nil
}

func (s *playerStore) onGet(ctx *node.Context) error {
	profile, err := s.profile(ctx, ctx.Request.UID)
	if err != nil {
		return err
	}
	return s.reply(ctx, profile)
}

func (s *playerStore) onAddExp(ctx *node.Context) error {
	var request shared.PlayerAddExpRequest
	if err := json.Unmarshal(ctx.Request.Body, &request); err != nil {
		return err
	}
	profile, err := s.profile(ctx, ctx.Request.UID)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	profile.Exp += request.Exp
	for profile.Exp >= profile.Level*100 {
		profile.Exp -= profile.Level * 100
		profile.Level++
		profile.Gold += 50
	}
	s.profiles[profile.UID] = profile
	s.mutex.Unlock()
	return s.reply(ctx, profile)
}

func (s *playerStore) profile(ctx context.Context, uid uint64) (shared.PlayerProfile, error) {
	s.mutex.Lock()
	profile, exists := s.profiles[uid]
	s.mutex.Unlock()
	if exists {
		return profile, nil
	}

	data, err := s.redis.HGet(ctx, playerDataKey, strconv.FormatUint(uid, 10)).Bytes()
	if err == nil {
		if err := json.Unmarshal(data, &profile); err != nil {
			return shared.PlayerProfile{}, err
		}
	} else if errors.Is(err, redis.Nil) {
		profile = shared.PlayerProfile{
			UID:   uid,
			Level: 1,
			Gold:  100,
		}
	} else {
		return shared.PlayerProfile{}, err
	}

	s.mutex.Lock()
	s.profiles[uid] = profile
	s.mutex.Unlock()
	return profile, nil
}

func (s *playerStore) reply(ctx *node.Context, profile shared.PlayerProfile) error {
	body, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return ctx.Reply(body)
}

func (s *playerStore) persistLoop(done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := s.flush(ctx); err != nil {
				logx.Errorf("停机持久化玩家数据失败: %v", err)
			}
			cancel()
			return
		case <-ticker.C:
			if err := s.flush(context.Background()); err != nil {
				logx.Errorf("定时持久化玩家数据失败: %v", err)
			}
		}
	}
}

func (s *playerStore) flush(ctx context.Context) error {
	s.mutex.Lock()
	profiles := make([]shared.PlayerProfile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		profiles = append(profiles, profile)
	}
	s.mutex.Unlock()

	if len(profiles) == 0 {
		return nil
	}
	values := make(map[string]any, len(profiles))
	for _, profile := range profiles {
		data, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		values[strconv.FormatUint(profile.UID, 10)] = data
	}
	if err := s.redis.HSet(ctx, playerDataKey, values).Err(); err != nil {
		return err
	}
	logx.Infof("玩家数据持久化完成 count=%d", len(profiles))
	return nil
}
