# diff

业务定义 Go 模型，生成器生成运行时对象及其全量、增量 Proto。内部变更路径只在进程内使用，不作为客户端协议。

## 定义与生成

模型文件使用 `//go:build diff_fast`，运行时代码使用 `!diff_fast`。字段的 `diff` 标签是 Proto 字段编号，范围为 1..999；`diff:"-"` 保留运行时字段及原始标签，不参与同步。

```go
//go:build diff_fast

package player

type Player struct {
    Uid   uint64           `diff:"1" bson:"_id"`
    Level int32            `diff:"2" bson:"level"`
    Items map[uint64]*Item `diff:"3" bson:"items"`
}

type Item struct {
    Count int32 `diff:"1"`
}
```

```sh
go run github.com/2comjie/nova/cmd/diff-gen -dir ./player -proto-dir ./proto
```

运行时代码输出到模型旁的 `*_diff.gen.go`；Proto 输出到 `proto/<包名>/<源文件名>.proto`；Proto 的 Go 包路径为所属模块的 `pb/<包名>`。随后用 protoc 生成所需语言的代码。可运行示例见 `examples/diff_app`，执行 `make run` 会完成生成、编译和运行。

支持基础字段、对象指针、基础 Map、对象 Map、基础 Slice、对象 Slice。对象引用应形成无环图；同一对象可以有多个父引用，每个引用路径都会收到变化。Proto 按值传递，不保留对象共享身份。

`type A int32` 等命名基础类型及 `type B = A` 别名会保留在运行时字段和方法签名中，Proto 按底层基础类型编解码，不额外生成 Proto enum。定义在 `diff_fast` 文件里的基础类型、别名及 const 声明会复制到生成代码；定义在普通 Go 文件里的声明直接复用。自定义方法请放在普通 Go 文件中。

`time.Duration` 按 int64 纳秒处理，保留 Go 的原生精度。`time.Time` 及其别名支持字段和集合元素，Proto、JSON、BSON 均使用 int64 毫秒时间戳。解码使用 UTC，不保留单调时钟、时区身份和不足毫秒的精度；字段、Map 值和 Slice 元素按毫秒比较变化。时间不作为 Map key。

时间整数 0 表示 Unix 起点，缺失的时间整数也按 0 解释。Go 的 `time.Time{}` 编码为其实际毫秒值 `-62135596800000`，不是 0，能够正确往返。

## 全量与增量

```go
source := new(Player)
source.LoadSnapshot(full) // full 非空；恢复数据及父子链接，不产生增量
writer := diff.NewWriter()
source.InitLink(writer)

source.SetLevel(20)
update := source.Commit() // 返回独立 Proto，清空本轮 Writer

replica := new(Player)
replica.LoadSnapshot(full)
replica.Merge(update)
```

- `Snapshot()` 深复制业务值，不改变 Writer。
- `LoadSnapshot()` 建立或重置全量基线，保留 `diff:"-"` 字段；清除旧对象的父子链接，建立新链接。不清空已有 Writer，调用方应先处理重置前的待提交变化。
- `Commit()` 需要根对象已绑定 Writer。没有变更时返回空 Proto 消息，可用 `proto.Size(update) == 0` 判断，无需发送或推进同步版本。
- `Merge()` 按序应用增量，对象必须已有对应基线。接收副本通常不绑定 Writer；绑定后应用增量也会记录本地变化，不会自动清空 Writer。
- 字段编号、类型及生成配置错误在生成或编译阶段失败。基础设施配置错误直接 panic，不包装或储存错误。

对象及 Writer 在所属 Actor 内串行使用。提交得到的 Proto 与运行时对象独立，可以交给网络线程。交给同步历史的消息视为只读。

## 协议规则

每个类型复用同一个 Proto 消息：普通字段表示全量，附加字段表示增量。全量和增量由外层消息区分，不能把增量当全量加载。

| 字段类型 | `index + 1000` | `index + 2000` | `index + 3000` | `index + 4000` |
| --- | --- | --- | --- | --- |
| 基础字段 | optional update | | | |
| 对象指针 | set | clear | update | |
| Map | clear | set | delete | |
| 对象 Map | clear | set | delete | update |
| Slice | updated | update | | |

对象指针的 set、clear、update 属于同一个 oneof。Map 按 clear、set、delete、update 的顺序应用。Slice 使用 updated 标志加整段数组替换，空数组也能表达清空。

跨语言客户端应按上述规则应用字段，不能用 `proto.Merge` 或 C# `MergeFrom` 代替增量应用：它们不理解删除、清空和数组替换。生成的 C# 文件是 Proto 消息定义，客户端的运行时状态应用逻辑由客户端实现。

Proto 不区分空 Map/Slice 与 nil 集合；repeated message 的 nil 元素经过编码会成为空消息，不承诺保留 Go 的 nil 元素身份。对象 Map 不保存 nil 值，`Store(key, nil)` 表示删除。窄整数在 Proto 中使用 int32/uint32，外部输入应遵守业务类型范围。

## JSON / BSON

生成对象实现标准的 `MarshalJSON`、`UnmarshalJSON`、`MarshalBSON`、`UnmarshalBSON`，内部使用普通数据结构与标准库 / MongoDB Go Driver v2。对象字段递归调用子对象的编解码，不经过 Proto。

```go
data, err := json.Marshal(player)
err = json.Unmarshal(data, player)

document, err := bson.Marshal(player)
err = bson.Unmarshal(document, player)
```

生成项目需要依赖 `go.mongodb.org/mongo-driver/v2/bson`。JSON 和 BSON 分别遵循自己的字段标签、`-`、`omitempty` 等选项；`diff:"-"` 字段始终排除，并在解码时保留原值。

解码先进入临时数据结构，全部成功后才替换对象的基线，恢复父子链接，不产生增量、不清空已有 Writer。缺失字段按零值加载，Map/Slice 整体替换，不保留旧元素。JSON 根值 `null` 按空基线加载。对象 Map 中的 null 元素按不存在处理；对象指针和对象 Slice 中的 null 保留。

bool Map 的 key 使用 `diff.BoolKey` 的标准文本接口，输出 `"true"` / `"false"`。整数 Map key 由标准库转换为十进制字段名；`[]byte` 保留原生类型，JSON 使用 Base64，BSON 使用 binary。

BSON 无法表示大于 MaxInt64 的 uint64 值，编码直接返回驱动错误，不截断或改成字符串。BSON 对象指针的 `inline` 会在生成时拒绝，避免驱动展开运行时包装结构而丢失字段；使用普通嵌套对象即可。

编码应在对象所属 Actor 内串行执行，内部数据结构是同步编码视图，不是供后台线程持有的快照。得到字节结果后可以交给其他线程发送或保存。这里不包含 Mongo 连接和增量写入。

## 多客户端版本

```go
history := snap.NewManager[uint64](0, 128, source.Snapshot)
history.Bind(subscriptionId)
initial := history.Pull(subscriptionId) // 首次必定全量
// 发送 initial.Full 和 initial.Version，收到应用完成确认后：
history.Ack(subscriptionId, initial.Version)

source.SetLevel(21)
history.Append(source.Commit())
next := history.Pull(subscriptionId) // BaseVersion、Version 和顺序增量
```

`Bind` 新建订阅；`Resume` 从已知有效版本恢复；`Unbind` 移除订阅。首次全量未确认时继续提供全量，历史淘汰后自动回退全量。各客户端进度独立，Ack 不能超过向该客户端发送的版本。

订阅标识应区分重连前后，例如使用包含 Uid 和订阅代次的可比较结构作为 Client。版本仅在同一对象世代内有效，进程重启或对象重建后应重新建立全量基线。传输层负责携带对象/订阅标识、基线版本和目标版本，接收方检查顺序后调用 LoadSnapshot 或 Merge。

这里不包含 AOI、Gate 推送策略和数据库保存调度。
