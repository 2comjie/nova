# TCP RPC

服务间使用 TCP 长连接和 Protobuf；不使用 gRPC，不支持流式调用。不改变 Gate 对游戏客户端的协议。

## 业务接口

继续在 `.proto` 中定义普通 `service`，通过 `protoc-gen-go-rpc` 生成 `_rpc.pb.go`。业务实现返回 `(*Rsp, *rpc.Error)`；生成客户端返回 `(*Rsp, error)`，因为连接、超时和编解码也可能失败。

```go
func (s *Service) Ping(ctx context.Context, req *pb.PingReq) (*pb.PingRsp, *rpc.Error) {
    return &pb.PingRsp{}, nil
}

server := rpc.NewServer()
pb.RegisterPlayerServiceServer(server, implementation)
// 注册在 Serve 前完成，重复注册直接覆盖。
// server.Serve(listener) 由 Node / Gate 生命周期启动。

conn := rpc.NewConn("127.0.0.1:9001")
defer conn.Close()
client := pb.NewPlayerServiceClient(conn)
rsp, err := client.Ping(ctx, &pb.PingReq{})
```

已有 `rpc/client.Client` 也可以传给生成客户端，保留服务发现、负载均衡、指定节点和 Actor 路由。

业务错误直接传输 `rpc.Error{Code, Message, Detail}`，不嵌套原始 error，不放进 context，也不自动转发给游戏客户端。框架错误使用 `0xffff0000` 高位区间，业务不要占用该区间。

## 调用语义

- 同一连接支持多个并发调用，用 Seq 匹配响应；普通业务方法可能并发执行，Actor 方法仍进入自己的 mailbox 串行执行。
- `Invoke` 等待响应；`Conn.Send` 只确认写出，不确认远端执行。Actor `Tell` 仍等待远端入队确认，与 `Send` 不同。
- 默认调用超时 10 秒，服务端处理上限 30 秒；可用 `WithCallTimeout` / `WithRequestTimeout` 调整。调用的剩余时间传到服务端。
- 主动取消只结束本地等待，没有远端取消消息；服务端由超时或断线取消 context。业务执行过程中需要自己响应 context，超时不代表未执行。
- 断线结束在途调用，下次调用重新连接；不自动重放请求。Actor 原有“未执行并返回重定向”的一次路由纠正保留。
- `Shutdown(ctx)` 停止接收新调用，等待在途响应写出；超过关闭期限则断开连接并取消处理 context。
- 默认最多 1024 个待响应调用、1024 个并发处理；超出返回 `CodeBusy`，可用 `WithMaxPending` / `WithMaxConcurrent` 调整。

## 协议

复用 `packet.Codec` 的 20 字节大端包头，默认整帧上限 4 MiB：

| 偏移 | 长度 | 内容 |
| --- | --- | --- |
| 0 | 2 | Magic：`0x822f` |
| 2 | 2 | Type：请求 `1`，响应 `2` |
| 4 | 4 | 整帧长度，包含包头 |
| 8 | 4 | Route：内部 RPC 固定 `1` |
| 12 | 8 | Seq：请求与响应对应，`0` 表示 Send |

包体使用 `proto/rpc/error.proto` 中的 `Request` / `Response`：请求携带完整方法名（例如 `/taoxi.player.service_rpc.PlayerService/Ping`）、业务 Protobuf 字节及剩余超时纳秒；响应携带业务 Protobuf 字节或一层 `Error`。

默认 TCP 用于受信任内网。跨不可信网络时，通过 `WithTCPOptions(netTcp.WithTLS(...))` 和 `WithServerTCPOptions(netTcp.WithTLS(...))` 配置 TLS，必要时使用双向证书验证。
