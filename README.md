# Wali

Wali 是一个使用 Go 编写的分布式游戏服务器框架，负责客户端长连接、玩家会话、
请求路由、Node 间 RPC、状态定位、服务发现和应用生命周期，让业务代码专注于聊天、
玩家、房间、匹配和地图等游戏逻辑。

> 项目目前处于基础框架阶段，API 和协议仍可能调整。

## 环境要求

- Go 1.24.3 或更高版本
- Redis（运行完整示例时需要）

## 安装

在你的 Go 项目中按需安装 Wali 包：

```bash
go get github.com/2comjie/wali/packet@latest
```

也可以将 `packet` 替换为需要使用的包，例如：

```bash
go get github.com/2comjie/wali/network@latest
go get github.com/2comjie/wali/app/node@latest
```

## 核心模块

- `packet`：网络包格式、字段校验和池化编解码。
- `network`：连接、Session、Bind、心跳、Call、Tell、Push 和生命周期。
- `app/gate`：客户端会话、配置路由、Filter 和 Node 转发。
- `app/node`：业务 Router、Middleware、Handler、Component 和 Gate Push。
- `registry`：服务注册与发现。
- `locator`：玩家 Gate 定位和游戏状态 Node 定位。
- `deploy`：Gate、Node 及其基础设施的组装和启动。

## Packet

客户端协议包含 `Req`、`Rsp`、`Push`、`Ping`、`Pong`、`BindReq` 和
`BindRsp` 七种包类型。固定包头为 20 字节：

```text
+----------+----------+------------+------------+----------------+
| Magic(2) | Type(2)  | Length(4)  | Route(4)   | Seq(8)         |
+----------+----------+------------+------------+----------------+
| Body (Length - 20 bytes)                                      |
+----------------------------------------------------------------+
```

从 `Codec.Read` 得到的消息和 `Codec.Encode` 返回的缓冲区使用内存池管理，调用方在
使用完成后必须调用 `Release`。

## 运行示例

先启动 Redis：

```bash
docker run --rm --name wali-demo-redis -p 6379:6379 redis:7
```

然后参考 [Gate + Chat Node + Player Node 示例](examples/game/README.md) 分别启动
Gate、Chat Node、Player Node 和客户端。

## 开发

运行全部测试：

```bash
go test ./...
```

涉及并发和生命周期的修改还应运行：

```bash
go test -race ./...
```

更完整的领域术语和架构约束见 [CONTEXT.md](CONTEXT.md)。
