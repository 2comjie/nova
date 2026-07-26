# Wali 领域上下文

## 项目目标

Wali 是分布式游戏服务器框架。它负责客户端长连接、玩家会话、请求路由、Node 间 RPC、
状态定位、服务发现和 App 生命周期，让业务代码集中处理聊天、玩家、房间、匹配、地图等
游戏逻辑。

Wali 当前处于基础框架阶段，已经能跑通 Gate → Node → Gate 的请求和推送链路，但还没有
覆盖生产游戏服务器所需的全部可靠性、可观测性和运维能力。

## 领域术语

### Packet

客户端连接上的二进制网络包。类型固定为 Req、Rsp、Push、Ping、Pong、BindReq、BindRsp。
Packet 不包含压缩、加密、认证和业务序列化策略。

### Session

一个客户端连接在 Gate 上的会话。Session 使用 Gate 进程内自增 ID 标识，只在当前 Gate
内有意义，不发送给客户端，也不要求跨 Gate 全局唯一。

Session 在 Bind 成功前不合法；超过 BindTimeout 仍未认证必须关闭。Bind 成功后 Session
持有 UID，并通过 GateLocator 建立玩家位置。

### Call

需要响应的客户端请求。Packet Req 的 Seq 大于零，Gate 调用 Node.Call，业务 Handler
可以通过 `Context.Reply` 返回 Body。

### Tell

不需要响应的客户端请求。Packet Req 的 Seq 为零，Gate 调用 Node.Tell，业务 Handler
不能调用 `Context.Reply`。

### Push

Node 主动向玩家客户端发送消息。Node 先通过 GateLocator 找到玩家所在 Gate，再定向调用
Gate.Push。

### Gate

面向客户端的网关 App。Gate 管理 Session、Bind、心跳、UID 定位、配置路由、Filter 和
Node RPC 转发。Gate 不实现聊天、玩家、背包、匹配等具体游戏业务。

### Node

承载游戏业务的 App。Node 使用 Router、RouteGroup、Middleware 和 Handler 处理请求，
并通过 Proxy 使用 Push、Kick、Bind、Unbind、Locate 等受限能力。

Node 的 ServiceName 描述可负载均衡调用的一类实例，例如 `chat`、`player`、`room`。

### Route

稳定的 `uint32` 业务请求编号。Gate Route 将客户端 Route 映射到 Node 目标策略；Node
Router 将 Route 映射到业务 Handler。

Route 在启动阶段注册并通过 Freeze 编译，运行阶段只读。

### Filter

Gate 转发前后的请求处理逻辑，类似 Spring Cloud Gateway Filter。Filter 可以校验、
限流、重写请求、修改目标或终止转发。

### Middleware

Node Handler 的调用链包装器，用于鉴权、日志、追踪、恢复和业务前置处理。

### Binding

一个游戏状态归属关系，由 `binding + key → Node instance ID` 表示。例如：

- `team + team-100 → room-2`
- `room + room-9001 → room-5`
- `player + user-1 → player-3`

Binding 不与 ServiceName 固定绑定。新增状态类型不需要新增 App 类型。

### GateLocator

保存 `UID → Gate instance ID`，用于 Node Push 和 Kick。

### NodeLocator

保存 `binding + key → Node instance ID`，用于有状态业务的定向路由。

### ServiceInstance

Registry 中的可调用进程实例，包含 ID、ServiceName、RPC 地址、权重、状态和元数据。
ServiceInstance 不携带 Route 配置。

### Component

由 Node 管理生命周期的业务模块，例如 MatchMaker、地图循环、玩家存储和房间管理器。
Component 按注册顺序启动、按相反顺序关闭。

### Deploy

Gate 和 Node 的直接启动入口。Deploy 启动配置中心、创建 RPC Server/Client、生成实际
RPC 端口、组装 Locator 和 Registry，并处理进程信号与资源释放。

## 核心调用链

### 客户端请求

```text
Client Req
  → Gate Session
  → Gate Route + Filter
  → RPC Client选择Node
  → Node Router + Middleware
  → Handler
  → 可选Rsp
```

### Node Push

```text
Node Handler
  → Proxy.Push
  → GateLocator定位UID
  → 定向Gate.Push
  → Session Push
  → Client
```

### 优雅停机

```text
注销ServiceInstance
  → 停止新流量
  → 排空RPC
  → 关闭App.Done
  → 反向关闭Component
  → 等待AddWait任务
  → 关闭Config、Discover、Locator、Registry
```

## 已确定的设计

- Packet 不划分 V1、V2，不包含 Version、Flags 和 PacketID。
- Packet 不包含压缩和加密标志。
- Req、Rsp、Push 的业务 Body 保持透明。
- 压缩和加密位于 Network 层，通过 Option 注入。
- Gate Route 来自配置中心，不来自 ServiceInstance。
- Node Handler 自己决定是否 Reply；Gate 根据客户端 Req 的 Seq 选择 Call 或 Tell。
- Session ID 是 Gate 进程内自增值，不返回客户端。
- App Proxy 直接引用 App，只暴露允许业务使用的方法。
- Network 对上层只提供 SessionStart、SessionEnd、SessionBind、Heartbeat、Req 五类 Hook。

## 当前缺口

- 服务健康检查、摘流和就绪状态。
- 请求超时预算、重试边界、熔断和过载保护。
- 玩家/房间等有状态对象的迁移、租约、并发所有权和故障恢复。
- 统一错误码及客户端错误响应。
- Metrics、Tracing、结构化访问日志和管理端点。
- 配置变更的路由热更新策略。
- TLS/WSS/KCP 生产部署模板及密钥轮换。
- 跨进程集成测试、故障注入和容量基准。
