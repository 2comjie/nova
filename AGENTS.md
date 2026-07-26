# Wali 项目提示词

Wali 是一个使用 Go 编写的分布式游戏服务器框架。修改代码前先阅读根目录的
`CONTEXT.md`，涉及架构决策时再阅读 `docs/adr/`。

## 沟通

- 使用中文沟通，代码注释使用中文。
- 先给结论，再解释关键设计和取舍。
- 不为显而易见的逻辑增加短小的转发函数或 Helper。
- 不确定的需求先从现有代码和测试中寻找答案，不另起一套风格。

## 架构约束

- `packet` 只负责包格式、字段校验和池化编解码，不负责压缩、加密、认证和连接。
- 压缩通过 `network.Zipper` 注入，加密通过 `network.Cryptor` 注入，只处理业务 Body。
- `network` 负责连接、Session、Bind、心跳、Call、Tell、Push 和生命周期。
- Gate 负责客户端 Session、配置路由、Filter 和 Node 转发，不处理具体游戏业务。
- Node 负责业务 Router、Middleware、Handler、Component 和 Gate Push。
- Route 信息配置在 Gate，不放进 `endpoint.ServiceInstance`。
- Binding 表示游戏状态归属，不与 ServiceName 绑定；新增游戏状态不应要求新增 App 类型。
- `WithNode` 是业务要求的强制定向调用，不校验目标服务名。
- BindReq 可以使用 protobuf；Req、Rsp、Push 的 Body 对业务保持透明。
- RPC 生成代码不向 Client 额外传递 serviceName，保持当前 Locator 插件设计。

## 编码约束

- 不使用 `iota` 定义协议值、状态值和需要长期稳定的常量。
- 不为内部必填依赖添加无法恢复的 `nil` 分支；构造阶段校验，运行阶段依赖不变量。
- 后台协程使用 `help.SafeGo`，需要隔离 panic 的同步回调使用 `help.SafeRun`。
- Packet 和网络读写优先使用 `core/buffer` 内存池，并明确所有权与 `Release` 时机。
- 不修改生成的 `*.pb.go` 和 `*_grpc.pb.go`；修改 proto 后使用生成脚本。
- 不在日志中输出 Bind token、密钥或完整敏感 Body。
- 保持 `Shutdown` 幂等；后台任务启动前调用 `AddWait`，退出时调用 `DoneWait`。
- 不保留无实际用途的字段、接口、Flags、版本层或兼容层。

## 验证

- 修改包后先运行对应包测试。
- 跨模块修改运行 `go test ./...`。
- 并发和生命周期修改运行相关包的 `go test -race`。
- 协议修改补 golden test、截断测试和 fuzz。
- 不为了让检查通过而修改无关的用户代码。

## Agent skills

### Issue tracker

Issues 和 PRD 使用仓库 `github.com/2comjie/wali` 的 GitHub Issues。详见
`docs/agents/issue-tracker.md`。

### Triage labels

使用 `needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、
`wontfix` 五类标签。详见 `docs/agents/triage-labels.md`。

### Domain docs

本仓库采用单一上下文：根目录 `CONTEXT.md` 保存领域语言，`docs/adr/` 保存架构决策。
详见 `docs/agents/domain.md`。
