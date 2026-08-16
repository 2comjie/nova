# Gate + Chat Node + Player Node 演示

这个例子展示一个 Gate、两个 Node 和客户端如何使用 `deploy`：

- Gate 监听客户端 TCP 连接，通过配置中心的 Route 转发请求。
- Chat Node 处理聊天请求，并通过玩家所在 Gate 推送给目标玩家。
- Player Node 处理养成数据查询和经验升级。
- Player Node 使用 `AddWait`、`DoneWait` 和 `Done` 管理 Redis 持久化协程。
- Registry、Discover、Locator 都使用本地 Redis。

## 启动

先启动本地 Redis：

```bash
docker run --rm --name nova-demo-redis -p 6379:6379 redis:7
```

然后在 `examples` 目录打开多个终端。

启动 Gate：

```bash
go run ./game/gate
```

启动 Chat Node：

```bash
go run ./game/chat
```

启动 Player Node：

```bash
go run ./game/player
```

启动接收聊天的玩家：

```bash
go run ./game/client --uid=user-2
```

再启动另一个玩家，并给 `user-2` 发消息：

```bash
go run ./game/client \
  --uid=user-1 \
  --to=user-2 \
  --message="组队吗？"
```

客户端启动后还会调用 Player Node 查询养成数据、增加经验并触发升级。

## 常用参数

```text
Gate:
  --listen=127.0.0.1:8000
  --config=./game/config/gate.yml
  --service=gate
  --id=gate-1
  --redis=127.0.0.1:6379

Chat Node:
  --service=chat
  --id=chat-1
  --rpc-listen=127.0.0.1:0
  --redis=127.0.0.1:6379

Player Node:
  --service=player
  --id=player-1
  --rpc-listen=127.0.0.1:0
  --redis=127.0.0.1:6379
```

RPC 端口传 `0` 时由操作系统自动分配，`deploy` 会把真实端口注册到 Registry。
