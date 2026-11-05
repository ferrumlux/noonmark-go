本文件保留独立候选阶段的设计与验证记录。候选现已复制到正式 `internal/inputdraft`，import 已调整；正式整仓验收以本任务 VERIFY.md 为准。尚未接 UI。

# IME 草稿自动保存 gate：隔离 Go 移植设计

## 需求

将上游 `IMETextDraftAutosaveGate` 忠实移植成无副作用的 Go 状态机，作为将来 700 ms 标题自动保存的前置组件。保留 owner、generation、revision、schedule sequence、request ID、composing 屏障、显式 flush、单一保存请求、保存期间新增编辑、有限指数重试及 acknowledge/discard 行为。零值、nil receiver 及不合法构造必须 fail-closed。

本次为 A 级隔离增量。只修改 `input-draft-work/`，不改正式 `noonmark-go/` 或原上游；不 commit、不 push。只使用内存中的合成 fixture，不接触 production、iCloud、Keychain、AI、网络或用户资料。

## 现状

- 已阅读上游 `AGENTS.md` 及正式移植边界 `noonmark-go/ports/mygo/AGENTS.md`。本轮明确授权的新隔离目录优先于上游针对另一台 Linux 主机的旧固定路径。
- Ground truth 是公开仓库 `quboliu/noonmark`，commit `43e6eefaae3a04be89cab3e234d470111afd80ef` 的 `Sources/NoonmarkMacRuntime/IMETextDraftAutosaveGate.swift` 和 `Tests/NoonmarkMacRuntimeTests/IMETextDraftAutosaveGateTests.swift`。
- 上游官方测试共有 9 项，其中 8 项属于本 gate；另一项针对独立的 `IMETextBindingPublicationPolicy`，不属于本次移植范围。
- 当前只有 Linux Go toolchain，没有 Swift 或真实 Mac/MyGo 输入法运行面。纯状态机测试不等于 Swift 动态 oracle，也不能证明原生输入法、标题组件、生命周期 flush 或存储写入已经接通。
- 标题编辑器的 multiline、`nonemptyTrimmed` 及生命周期事件在调用方，本 gate 不处理文本，不验证标题，不管理定时器，不读取或写入存储。

## 设计

### API 与隔离

新增独立 module `noonmark-input-draft-candidate`，包为 `internal/inputdraft`。初始 stdlib-only 约束于 2026-11-05 经集成阶段明确更新：只为保留 Swift Unicode canonical equality 引入已在正式模块锁定的 `golang.org/x/text v0.43.0`，最低 Go 1.26.0。导出 `Gate`、可比较的 `Schedule`、可比较的 `PersistenceRequest`；构造为 `New(ownerID)` 和 `NewWithGeneration(ownerID, generation)`，返回 `(*Gate, error)`。空 owner 或初始 generation 为 0 时返回 nil 和 error；不擅自 trim owner，空白 owner 与上游一致合法。

所有状态私有。`initialized` 区分无法合法构造的零值与从最大 generation wrapping 到 0 的合法 gate。零值或 nil receiver 的查询为安全空状态，事件不会生成 schedule/request。Gate 可按值复制，内部 in-flight request 保存值而非共享可变指针，调用方修改返回值不能篡改 gate。

gate 按调用方单一串行事件流推进，不添加锁、goroutine、timer、storage 或 network。需要跨 goroutine 时，由调用方串行化整个事件/快照/持久化发起流程；`-race` 测试不代表并发调用同一实例合法。

### 源码状态转换

- 真正文本变化：revision wrapping +1，retry 清零；composition 状态有变或文本有变均使旧 schedule 失效。非 composing 且 dirty 时发出 700 ms schedule，已有 explicit flush 则为 0 ms。
- `RequestFlush` 总会设置 sticky flush；只有非 composing、dirty、无 in-flight 时返回 0 ms schedule。
- `PermitsAutosave` 同时比对 owner、generation、revision、sequence，要求非 composing、dirty、无 in-flight。delay 不属于比对条件。`BeginPersistence` 必须经相同 gate 检查，再推进 request ID。
- 成功只接受精确匹配的 in-flight request；acknowledged revision 使用 unsigned `max` 而不是 modulo 顺序。仍 dirty 时返回下一 schedule；干净时清 flush 并 invalidate。
- 失败只接受匹配 request；释放 in-flight，retry wrapping +1；非 composing 且 dirty 时依次发出 1000/2000/4000/8000/10000 ms，之后维持 10000 ms。
- acknowledge 无持久化确认当前 revision、清 retry/flush/in-flight、invalidate，但不结束 composing。
- discard 推进 generation（wrapping）、重置 revision/ack/retry/flush/composing/in-flight、invalidate；schedule 和 request sequence 不重置。

### 必须保留的源码细节

1. 保存期间的新编辑可能返回 schedule，但在 in-flight 结束前不能获得许可；成功回调会生成较新的 schedule。
2. 成功回调在仍 dirty 且 composing 时也会返回 schedule。它仍被 `PermitsAutosave` 拒绝，composition 结束会重排。
3. 干净状态的 flush 仍为下一次编辑留下 sticky flush；重复 flush 会使先前 schedule 失效。失败重试不会因 sticky flush 变为 0 ms。
4. `acknowledgedRevision = max(...)` 在 revision wrapping 后不作纠偏；不得加入“更合理”的环形版本排序。
5. Swift 的 `&+=` wrapping 与普通 `retryAttempt - 1` 不同。retry 从 `UInt64.max` wrap 成 0，且进入重试计算分支时，原版会 underflow trap。Go 用明确 panic 对应 fail-closed，不能悄悄变成 10 秒或 1 秒重试；若 composing 或已 clean 则不会执行减法，也不会 panic。此状态正常使用几乎不可达，但仍测试。
6. owner 是路由标识，不是安全凭证。两个 gate 以相同 owner/generation 创建，可能产生相同 token；调用方必须保证 owner/generation 生命周期隔离。

### 验证计划

移植 8 项官方 gate 测试；增加全部 guards 的 table tests、精确 counter/sequence 检查、无变化事件、重复/伪造/过期 token、返回值与 gate 副本隔离、保存期间新增编辑、所有重试 reset 路径、ack/discard、uint64 wrapping/underflow、无效实例 fail-closed，以及有明确深度上限的状态空间遍历。验证不能声称穷举 2^64 状态。

使用 `../go/bin/go`，显式外置 `GOPATH=/tmp/noonmark-gopath`、`GOCACHE=/tmp/noonmark-gocache`；执行 gofmt、`go test -race`、`go vet`、coverage。证据只来自本候选 module，不能代替正式仓库全量门禁或原生 UI/IME 验收。

## 集成与回滚

本轮不集成。候选完成后，由集成审阅后复制包至正式 module；调用方应按事件处理 schedule，等待 delay 后重新调用 `BeginPersistence`，并把原 request 原样交回成功/失败事件。schedule 存在不等于获准写入。撤销本增量只需不导入候选包，正式路径没有变化。

### Unicode 比较设计修订（修订先于代码）

实施审查发现 Go string 原字节相等不能对应 Swift String canonical equivalence。集成阶段明确要求消除该差异，撤回这个必要范围内的 stdlib-only 约束；使用正式模块已锁定的 `golang.org/x/text v0.43.0`，最低 Go 1.26.0。此决定沿用正式代码 GO-RUNTIME-006 的比较原则。

owner 在 schedule 许可和 request 全字段匹配时以 NFC 比较，数值字段仍逐项精确相等。先做字节相等 fast path，否则比较双方的 NFC。仅用于比较，不改 gate、schedule 或 request 保存/返回的原始 owner 字节，不 trim，不拒绝非 ASCII。新增预组合/分解双向、不同 owner、callback owner canonical equality 与数值身份不匹配测试，并重跑完整 race/vet/coverage。不得将 Go struct 的内建 `==` 当作 Swift owner-aware 许可或 request 匹配；调用方应使用 gate API。
