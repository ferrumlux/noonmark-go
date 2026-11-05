此为隔离候选评审时的完整记录。正式已复制 session 两文件及独立回归，当前集成状态以 SESSION.md 为准。以下历史候选路径不可当正式构建入口。

# 标题草稿 session 候选结果

日期：2026-11-05。状态：纯候选完成，未正式集成，UI 仍不开放。

## 结果

候选目录为 `noonmark-mygo-work/title-session-work/`。完成文本拥有的 session、不可修改作业/保存请求、实例和 generation 防串线、真实确认字节回传、nonemptyTrimmed 与事件驱动生命周期状态。没有改正式 `noonmark-go`、上游 `noonmark-reference` 或 `mygo`；没有 commit/push。独立修改的 Service 不属于本候选变更。

29 项 session 测试及 36 项复制的 Gate 测试通过；额外有界串行事件序列包含 96 × 160 = 15,360 次事件，检查失败/未核实结果不修改文本、不能并行保存、旧完成回调不改变状态，以及 dirty/in-flight/composing/error/待执行作业不可能返回 FlushClean。所有 fixture 均为合成文字。

## 文件清单

实际新文件：

- `DESIGN.md`：先于实现写出的需求、现状、设计及评审更新。
- `RESULTS.md`：本报告。
- `TEST-OUTPUT.txt`：最终完整 `scripts/check` 运行输出。
- `go.mod`、`go.sum`：独立 module；仅锁定正式已有的 `golang.org/x/text v0.43.0`。
- `titlesession/session.go`：生产候选实现。
- `titlesession/session_test.go`：29 项 session 场景/安全不变量测试。
- `internal/inputdraft/gate.go`：正式 Gate 原文副本，只供隔离编译。
- `internal/inputdraft/gate_official_test.go`。
- `internal/inputdraft/gate_state_test.go`。
- `internal/inputdraft/gate_test.go`。
- `internal/inputdraft/gate_unicode_test.go`。
- `internal/inputdraft/gate_wrap_test.go`。
- `scripts/check`：无网络、外置 Go 缓存的重跑入口。

Gate 实现 SHA-256 与正式文件一致：

`10d687630c9a70f59a64eb8eac0b31b47085190d2b78b46944b3130c52acc210`

复制的 Gate 测试只调整 import module path。将来正式集成只取 `titlesession` 实现/测试并改其 Gate import；**不要复制整个 Gate、副本测试、候选 go.mod/go.sum 或构建脚本到正式仓库**。

## API

- `New(Config) (*Session,error)`：固定 owner、session ID、初始 generation、trace ID、已核实初始 persisted text。Session 只能通过指针串行使用，不可复制。
- `Identity()`、`State()`、`ScheduledJob()`：返回值快照或独立副本。Identity/Job/SaveRequest 内部字段私有，getter 不暴露可修改内部对象。
- `NativeSnapshotDidChange(identity, NativeSnapshot) Update`：接收真实 committed text/composing，不替原生控件读取、完成或猜测 marked text。重复同值观察保留既有 deadline。
- `RequestFlush(identity) Update`：结束编辑请求 canonicalization/0ms flush；本身不具备 native 最终化证据。
- `RequestLifecycleFlush(identity, snapshot, nativeFinalized) Update`：显式处理生命周期。布尔值是**调用方对原生最终化的确认**，组件不能验证它；之后输入/合成变化使此确认失效。
- `BeginPersistence(job) BeginResult`：再次验证候选；若可写，返回不可变 `SaveRequest`，其中有 Identity、GateRequest、TraceID、RawDraft、NormalizedText。空白普通 debounce 不发请求；空白结束编辑在无 in-flight 时恢复 ack。不能把存在 Job 当作保存许可。
- `PersistenceDidComplete(request, PersistenceResult) Update`：错误交给 Gate retry，保留文本。成功必须提供 `VerifiedText`；若没有则保留 in-flight，等待核实，不能自行使用 normalized 冒充落盘字节。核实值与请求必须 canonical 等价。
- `SynchronizePersistedText(identity,text) Update`：接收调用方已核实的最新有序发布；pending echo 只消费 marker，其余更新仅在无编辑/合成/保存中时应用。
- `Dispose(identity) Update`：只接受所提供快照已 FlushClean 的 session，拒绝丢弃未保存输入。外层必须已经停止原 native 来源并取到最新快照。
- `NormalizeTitle(text) (string,bool)`：CRLF/CR→LF，Go Unicode TrimSpace，保留内部换行与原 canonical bytes；bool 表示非空。

`Update` 包含 Accepted、当前 Job、DraftReplacement、State、Err。`Update.SnapshotIsFlushed()` 同时检查 Accepted、Err 和生命周期结果；用于判断本次调用是否成功处理已知快照。不能只读被拒绝事件返回的旧 State.Lifecycle，那仍可能如实表示更早快照已 clean。每次返回的是**当前**作业，不是要求重新启动 timer 的命令。相同 Job 不能重设 deadline，Job=nil 则取消调用方旧候选。700 ms/重试时间由调用方计时；0 ms 在安全阶段直接调用 BeginPersistence，不要等 After(0)。

`DraftReplacement` 是模型/控件写回建议，不是新 native snapshot。身份在事件来源处捕获，不能将旧事件重新标记成新身份。blank discard 推进 generation，后续真实事件须获得新的绑定身份。

## 原子 Service 接法

现有 Service 已提供 `SaveTraceTitleInputAcknowledged(traceID,title) (string,error)`；本候选没有复制或修改 Service。外层的单一有序执行路径应：

1. 使用不可变 request 捕获的 trace 与 normalized text，不重新读取 selection。
2. 按正式规范完成自然日刷新，然后调用 acknowledged Service。
3. 将同锁回传的实际 title 放入 `PersistenceResult.VerifiedText`；错误只放 Err。
4. 在原 session 的串行事件流交还原 request；再处理返回 Job、replacement、视图刷新和错误。

示意，不是已接 UI 的代码：

```go
actual, err := service.SaveTraceTitleInputAcknowledged(
    core.DayTraceID(request.TraceID()), request.NormalizedText(),
)
result := titlesession.PersistenceResult{Err: err}
if err == nil {
    result.VerifiedText = titlesession.OptionalText{Text: actual, Present: true}
}
// 下句必须回到拥有 session 的串行事件流。
update := session.PersistenceDidComplete(request, result)
```

本候选不决定错误是否可重试。Service 的 stale/conflict/incomplete/closed 错误不能被外层当作普通临时故障无限盲重试。Gate 给 retry 候选不意味着重试该 handle 已获准或一定安全。

## 生命周期意义

状态有 Invalid、NotRequested、NeedsNativeFinalization、WaitingPersistence、WaitingChanges、Failed、FlushClean、Disposed。查询 `SnapshotIsFlushed()` 只说明**组件收到的快照**已经清理或得到持久化确认。没有名为 SafeToClose 的 API。本次事件是否成功，优先检查 `Update.SnapshotIsFlushed()`；被拒的旧身份事件不能用其携带的先前 clean State 冒充本次成功。

真正 Close/退出仍需调用方在主线程真正关闭回调取得最新 `FinishTextEditing` 结果，重新送原生快照并确认全部 session。不能用 State.Draft 或 DraftReplacement 构造所谓最终原生快照，不能让 FlushClean 跳过此门禁。UI/native finalization 尚未由本任务验证。

PendingPersistedText 是成功后等待同值发布的 echo marker；它不是待落盘文本，不阻止 FlushClean。正式 Service 已同步发布 snapshot；外层要刷新视图，不新增 Swift deferred-publishing 屏障。

## 保留的源码细节

- 普通候选 700 ms，显式 flush 0 ms，错误重试 1/2/4/8/10 秒；所有 token 与重试顺序由现有 Gate 决定。
- 普通空白留在本地，不写、不假确认；生命周期结束编辑才保证等 in-flight 明确结果后恢复 LastAcknowledgedText。普通 blur 在保存过程中清空的边界见 SESSION.md，不承诺自动恢复。
- 普通成功保留 raw draft。一个已 clean、但仍含边缘空白的 draft 后续 finishEditing 不会凭空创建保存或立即 trim；这是上游实际行为，测试明确保留。
- 更晚输入不会被旧保存成功覆盖。成功更新 ack，但 Gate 仍为后来 revision 提供新候选。
- Return 只是内部换行，没有 Return 提交事件。
- Swift String 风格 canonical equality 由 NFC **比较**提供；不全局 NFC 改写草稿、请求或已核实持久化字节。
- external 同值 pending echo 不覆盖新 draft；dirty/composing/in-flight 时其他 external 更新被忽略，调用方有需要须重新送最新数据，不缓存旧更新为未来权威值。

## 明确安全差异

这些不是声称逐行等价的隐形改动：

1. Swift UI 在 normalized==lastAcknowledged 时直接成功，本候选仍发 Service 请求，验证 trace、closed/blocked、磁盘 prefix；它可能是无 durable revision 的验证读取。
2. Swift editor 以 normalized 作为 ack，本候选使用已核实实际 bytes，保留 canonical no-op 原字节；无核实结果继续等待，错误不确认。
3. Session 增加非零大小实例 token，防止新实例误用同一 owner/session/generation 数值而接受旧 token。Dispose 只允许已确认快照；没有强制丢弃接口。
4. 从“变化通知”转为可每次 build 观察快照时，重复同值事件不取消既有 Job。真正 composition 状态变化仍 invalidates；这是适配观察频率所需的明确处理。
5. 保存成功时若此刻 composing，不写回 canonical replacement。不能在异步成功回调侵入正在进行的原生合成。
6. lifecycle 保存失败锁存 Failed，普通后台重试成功不能静默复活已取消 close；需新的显式生命周期请求。空白 retry 也不能在失败或未最终化时自动清掉草稿。
7. 外部同步明确排除 in-flight/awaiting-verification，不让其清掉 Gate 当前 request；源 Gate 在极端 uint64 wrapping 时可能出现 dirty=false 但仍 in-flight，不能借此销毁请求。
8. normalization 对齐正式 Go 标题命令，额外明确 CRLF/CR→LF；Foundation whitespace 精确对等仍未由 Apple oracle 证明。

## 实际验证

入口：在本目录执行 `./scripts/check`。外置 `GOPATH=/tmp/noonmark-gopath`、`GOCACHE=/tmp/noonmark-gocache`，`GOTOOLCHAIN=local`、`GOPROXY=off`。实际 binary 是 `../go/bin/go`，版本 `go1.27.3 linux/amd64`。

最终实际输出摘录（完整日志见 TEST-OUTPUT.txt）：

```text
== toolchain ==
go version go1.27.3 linux/amd64
== format ==
gofmt clean
== copied Gate hash ==
10d687630c9a70f59a64eb8eac0b31b47085190d2b78b46944b3130c52acc210  internal/inputdraft/gate.go
10d687630c9a70f59a64eb8eac0b31b47085190d2b78b46944b3130c52acc210  ../noonmark-go/ports/mygo/internal/inputdraft/gate.go
== race tests, no cache ==
ok  noonmark-title-session-candidate/internal/inputdraft  1.264s
ok  noonmark-title-session-candidate/titlesession          1.066s
== vet ==
go vet passed
== statement coverage ==
ok  noonmark-title-session-candidate/internal/inputdraft  2.342s  coverage: 100.0% of statements
ok  noonmark-title-session-candidate/titlesession          1.178s  coverage: 98.1% of statements
```

race 与 coverage 均使用 `-count=1`。未覆盖部分主要是构造后 Gate 不可能拒绝同一已验证参数、同一串行 Begin 中二次 guard 的防御分支，不人为破坏内部状态凑 100%。race 通过不意味着同一个 Session 可并发调用。

## 尚未验证 / 集成边界

- 没有 UI 接线、真实 timer/时钟、导航 registry、多窗口 coordinator、Close/Quit 执行器。
- 没有原生 snapshot/finalization 实现，未跑 MyGo native IME 或 Apple/Windows/Linux 实机。
- 没有 Swift 动态 oracle，不承诺 Foundation whitespace 的全标量对账。
- 没有在此 module 真正调用 Service/磁盘；此处只验证请求必须发出及确认结果的纯处理。真实 Service 原子 route/ack/故障门禁见正式 Service 的独立证据。
- 没有 production、iCloud、Keychain、AI、网络或用户资料。
- 候选可以独立 review；不能据此启用标题编辑 UI，或声称安全退出路径已完成。
