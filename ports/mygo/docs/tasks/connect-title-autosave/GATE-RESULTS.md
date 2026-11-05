本文件保留独立候选阶段的设计与验证记录。候选现已复制到正式 `internal/inputdraft`，import 已调整；正式整仓验收以本任务 VERIFY.md 为准。尚未接 UI。

# IME 草稿自动保存 gate：结果与集成清单

## 已完成

在全新隔离目录 `input-draft-work/` 完成纯 Go 候选包 `internal/inputdraft`。保存 owner、generation、revision、schedule sequence、request ID，按 Swift 源码实现 700 ms debounce、显式 flush、composing 屏障、单一 in-flight、保存期间的新编辑、1/2/4/8/10 秒失败重试、acknowledge 与 discard。没有定时器、goroutine、文本字段、存储、网络或 UI 接线。

本次仅创建本目录文件；没有修改正式 `noonmark-go/` 或原版 `noonmark-reference/`，没有 commit 或 push。所有测试资料为内存中的合成 ID，不触及真实资料。

## 验证

2026-11-05，Linux amd64，`go version go1.27.3 linux/amd64`。独立 module 声明 Go 1.26.0，直接依赖 `golang.org/x/text v0.43.0`，版本与校验和均沿用正式模块。初始 stdlib-only 约束经集成阶段明确修订，仅为保留 Swift String canonical equivalence 引入这个已有依赖；生产包还使用 stdlib `errors`。

已执行并通过：

```sh
export GOPATH=/tmp/noonmark-gopath GOCACHE=/tmp/noonmark-gocache GOPROXY=off GOTOOLCHAIN=local
../go/bin/gofmt -w internal/inputdraft/*.go
../go/bin/go mod tidy
../go/bin/go test ./...
../go/bin/go test -race -count=1 -coverprofile=/tmp/noonmark-inputdraft-coverage.out ./...
../go/bin/go vet ./...
../go/bin/go tool cover -func=/tmp/noonmark-inputdraft-coverage.out
../go/bin/gofmt -l internal/inputdraft/*.go
```

- 36 项顶层测试通过；源码 100.0% statement coverage（25 个函数均 100%）；gofmt 无未格式化文件。
- 移植官方测试中的 8 项 gate 用例。第 9 项属于独立 binding publication policy，没有把它冒充本包已实现。
- Latin 与 Hangul owner 预组合/分解形式双向通过 schedule 与成功/失败 request 比较；不同 owner 及数值身份不匹配仍拒绝；原 owner 字节保持不变。
- 无变化事件、全部 snapshot 布尔分支、每个 schedule/callback 身份 guard、所有 flush guard 组合、重复 flush、单一 in-flight、新编辑与旧回调、返回值/值复制隔离均有回归。
- 重试序列、文本重置、composition-only 保留 retry、成功/ack/discard 重置、callback 顺序与失败守卫均有回归。
- generation/revision/schedule/request/retry 的 uint64 边界及 Swift 普通减法 underflow panic 均有显式测试。
- 对 12 种事件的所有长度 5 序列执行有界穷举：248,832 条完整序列，共校验 271,452 次前态→后态转换；独立前态规则检查每次完整状态、schedule/request 输出和许可 predicate。
- 零值、nil receiver、空 owner、初始 generation 0 均安全拒绝；generation 从最大值 wrap 到 0 的合法实例不会误判为零值。

100% statement coverage 不代表穷举全部 uint64 状态或所有并发调度。Gate 必须由调用方串行访问；race 通过不表示并发调用同一实例安全。

## 源码中保留的非直觉行为

- 干净状态 `RequestFlush` 为下一次编辑留下 sticky flush；重复 flush 会更新 sequence。
- 保存期间新编辑仍可能产生 schedule；in-flight 期间不能 begin。
- 成功时若仍 dirty，即使 composing 也返回 schedule；许可检查仍会拒绝。
- failure 重试不因 pending explicit flush 变成 0 ms；composition 结束使用 700 ms 或 pending flush 0 ms，而不是继续上次 backoff delay。
- acknowledge 不结束 composing；discard 不重置 request/schedule counters。
- revision wrapping 后仍采用 unsigned `max` ack，不使用环形版本顺序。retry wrap 后只有进入减法分支才 panic，guard 先返回时不 panic。
- token 没有隐藏 instance nonce。不同实例若共用 owner/generation，可能生成相同 token，调用方必须隔离 session 生命周期。

## 明确未完成或未证明

- 未运行 Swift/XCTest 动态 oracle，未进行 macOS/MyGo 原生输入法或腾讯输入法测试。
- 未接标题编辑 UI、multiline、`nonemptyTrimmed`、blur/close/lifecycle flush、调度器、事务或真实持久化；不能据此声称 700 ms 标题保存已向用户交付。
- 未运行正式仓库全量门禁；未改正式路径。
- 离线 `go list -m all` 因 x/text 声明的工具用 module metadata 未缓存而失败，没有为此联网补齐。生产 package 的实际依赖已用 `go list -deps` 验证，非 stdlib 仅 `x/text/transform` 与 `x/text/unicode/norm`；`go mod tidy`、构建测试及 vet 均可离线成功。

## 精确 API

```go
type Schedule struct {
    OwnerID string
    Generation, Revision, Sequence, DelayMilliseconds uint64
}
type PersistenceRequest struct {
    OwnerID string
    Generation, Revision, RequestID uint64
}
type Gate struct { /* 私有状态 */ }

func New(ownerID string) (*Gate, error)
func NewWithGeneration(ownerID string, generation uint64) (*Gate, error)

func (*Gate) OwnerID() string
func (*Gate) Generation() uint64
func (*Gate) IsComposing() bool
func (*Gate) HasUnflushedChanges() bool
func (*Gate) HasPersistenceInFlight() bool
func (*Gate) DraftDidChange() *Schedule
func (*Gate) CompositionDidChange(isActive bool) *Schedule
func (*Gate) NativeSnapshotDidChange(textChanged, nextCompositionState bool) *Schedule
func (*Gate) RequestFlush() *Schedule
func (*Gate) PermitsAutosave(schedule *Schedule) bool
func (*Gate) BeginPersistence(schedule Schedule) *PersistenceRequest
func (*Gate) PersistenceDidSucceed(request PersistenceRequest) *Schedule
func (*Gate) PersistenceDidFail(request PersistenceRequest) *Schedule
func (*Gate) AcknowledgeWithoutPersistence()
func (*Gate) DiscardLocalChanges()
```

`New` 默认 generation 1。owner 以 NFC 比较但保存/返回原字节，不能用 Go struct 内建 `==` 取代 gate 的 owner-aware 许可/回调匹配。所有返回 nil 的 schedule/request 代表没有新指令，不能自行解释成“保存已成功”。Schedule 是候选而非许可，调用方在 delay 到期后必须调用 `BeginPersistence`，将返回 request 与真正提交的同一份文本快照绑定，再原样交回成功/失败回调；不能以 `DraftDidChange`/timer 已触发代替事务成功确认。外部落盘错误只调用失败事件，不能直接 acknowledge。

Gate 不知道文本内容、字段验证或空白语义。与原 UI 一样，需要在独立适配层处理这些职责。本目录不替适配层规定 UI 时序。

## 文件与后续复制边界

- `internal/inputdraft/gate.go`：唯一生产实现，可复制到正式 module 的同名包。
- `internal/inputdraft/gate_official_test.go`：8 项官方 gate 行为改编，以及 public API 的构造/零值验证。集成时将唯一 import 从候选 module 路径改成正式 module 路径。
- `internal/inputdraft/gate_test.go`：guard、时序、状态与 owner 边界。
- `internal/inputdraft/gate_wrap_test.go`：uint64 wrapping、ack max 和 underflow 行为。
- `internal/inputdraft/gate_unicode_test.go`：owner canonical equality 与原字节保存、数值身份拒绝。
- `internal/inputdraft/gate_state_test.go`：有界事件空间与独立前态规则校验。
- `go.mod`、`go.sum`：只供隔离测试，锁定 x/text v0.43.0，不覆盖正式 module 的依赖文件。
- `DESIGN.md`：实施前记录的需求、现状、设计，以及实施中确认的语言边界。
- `RESULTS.md`：本结果与 API。
- `ATTRIBUTION.md`、`LICENSE`：公开上游来源与 AGPLv3 许可，集成时保留 attribution。

## 来源指纹

精确上游 commit：`43e6eefaae3a04be89cab3e234d470111afd80ef`。

- `Sources/NoonmarkMacRuntime/IMETextDraftAutosaveGate.swift` SHA-256：`7c823e54cffc2656f4e674213e6407f4efa0d647189968f504567ea95eb5f501`
- `Tests/NoonmarkMacRuntimeTests/IMETextDraftAutosaveGateTests.swift` SHA-256：`b3e11ac10cd65c342bc6e957421a4532f2d2f3c5ab8e5a431aa6f7365f2a74f7`

源码链接与改编说明见 `ATTRIBUTION.md`。本候选尚未被正式集成，撤销时无需改变任何正式运行路径。
