# 子任务与任务链进度纯投影

- task-id：`subtask-progress-core`
- 日期：2026-11-05（UTC）
- 级别：A，纯领域只读投影；独立副本交付，不接 Service／UI。
- 上游：`43e6eefaae3a04be89cab3e234d470111afd80ef`。
- 状态：Go 合成 fixture 验证完成；Swift oracle **未运行**，不宣称完整复刻、跨平台实机验收或发行通过。

## 需求、现状与边界

现有 core 已有子任务编辑、完成、取消、废弃以及父任务完成命令。真实 UI 已显示真实 child，但未显示未经验证的加权进度。本轮只新增上游对应的只读 progress 与能力投影，交由后续独立审查决定是否整合。

工作副本为 `progress-core-work/ports/mygo`，由正式树 `noonmark-go/ports/mygo` 复制。只新增本文及 `domain-core/core/progress*.go`；既有 validator、commands、go.mod、Service、UI 和正式树均不修改。不 push、不运行生产身份，不读取 production／iCloud／Keychain，不调用 AI 网络。

fixture 只使用 development/e2e 合成资料。已移植命令能表达的流程经过真实 core commands；尚无对应 command 的 continuation、历史 floor 等用显式合成图表达，并通过原有 `ValidateTaskGraph`，不绕过或放松验证。

## API

```go
NewProgressProjection(graph TaskGraph) (*ProgressProjection, error)
(*ProgressProjection).SubtaskProgress(traceID DayTraceID) SubtaskProgress
(*ProgressProjection).TraceProgress(traceID DayTraceID) TraceProgress
(*ProgressProjection).CompletionCapability(traceID DayTraceID, today LocalDate) (DayTraceCompletionCapability, error)
(*ProgressProjection).CanSetManualProgress(traceID DayTraceID, today LocalDate) (bool, error)
```

- `SubtaskProgress`：`Total, Completed, Pending, Unfinished, Deferred, Abandoned int`，方法 `IsPartiallyCompleted()`、`CanCompleteParent()`。
- `TraceProgress`：`Mode` 及 `Percent, FloorPercent, CompletedWeight, TotalWeight int`。Mode 为 `ProgressManual`（`manual`）或 `ProgressWeightedSubtasks`（`weightedSubtasks`）。
- `DayTraceCompletionCapability`：成功查询后恰有 `Action` 或 `Blocker` 之一非空。`Action` 为 `CompletionComplete`／`CompletionUndo`；`Blocker` 为 `CompletionLockedDay`／`CompletionHistoricalDay`／`CompletionFutureDay`／`CompletionUnsupportedStatus`／`CompletionOpenSubtasks`。只有 unsupportedStatus 返回对应 `Status`。
- `CanSetManualProgress` 是上游 setter 守卫的布尔只读表示，不新增 setter、不接受目标百分比、不修改事实，也不保证未来执行时仍可操作。

构造先调用现有 validator，拒绝错误图且返回 nil projection；随后深复制输入。构造期间调用者不得并发改输入。构造后投影不可变，可并发读。输入变化后应在聚合状态成功更新时重建一次，不应每个 row 或每帧重建。后续 UI 应缓存于已确认 graph publish，不在渲染循环构造。零值或 nil receiver 按空图处理。

### 不存在 ID 与错误输入

上游 `traceProgress` 不抛错：不存在 ID 返回 manual，percent/floor/weights 全 0。本 API 保持这一点；已存在但无 child 历史、无 manual 值则为 manual 0%，weights `0/100`。

上游 `subtaskProgress` 对不存在 ID 返回全 0，`CanCompleteParent()` 因而为 true；这不是允许完成不存在父任务，真正能力查询仍检查实体存在、日期和状态。

`CompletionCapability`／`CanSetManualProgress` 对不存在或非法字符串 ID 返回 `ErrNotFound`，对 Go 可直接构造的无效 `LocalDate` 返回 `ErrInvalidInput`（先于 ID 查询）。本构造器主动验证的是整个 **bounded TaskGraph**；它并非“遇到坏记录忽略后继续计算”，更不是完整 snapshot／supplement／recurring-parent validator。不能把缺少上下文的导入资料直接交给它当作全量验收。

## 精确源语义与映射

### 当前 trace 的 child 计数

对应 `NoonmarkEngine.swift:2866–2878` 与 `Models.swift:1111–1136`。

只按 `Subtask.traceID` 计数，并排除 child 的 `cancelledDraft`。不按 lineage 去重；不改用 completed/count。部分完成为 `completed > 0 && completed < total - abandoned`。父完成的 child 条件只看 `pending == 0 && unfinished == 0`；`deferred` 和 `abandoned` 不阻挡父完成。

必须保留源中的细节：单独查询隐藏父 trace 的 `SubtaskProgress`，其 pending child 仍会计数。父 trace 是否显示是另一层契约，不能把这个查询当作 UI 可见性过滤器。

### weighted raw progress

对应 `NoonmarkEngine.swift:2880–2899, 3950–4002`，`Models.swift:79–99, 1138–1153`。

1. 父 trace 已 completed：raw 固定 manual 100%，weights `1/1`，即使 child 的加权百分比不满 100%。
2. 否则只取同 chain、`formsDayHistory`、日期不晚于目标日期的 trace，收集可呈现 child。
3. 按 lineageID 聚合，而非子任务 record ID、carried-from ID、当前 definition 或最新 child。每 lineage 的 weight 是这些记录的难度最大值，取值严格为 1／2／3。曾有任何一条 completed record，该 lineage 的**整个最大 weight**都计入 completedWeight。
4. abandoned child 保留在分母中，不自动当 completed；只有取消 child 不参加。取消 trace 下的 pending child 不参加。changed／returnedToPool 等虽不显示为 Day Todo row，仍形成历史，不得过滤。
5. 没有 lineage 才使用 trace.manualProgressPercent，nil 视为 0，weights `manual/100`。
6. weighted 百分比使用 Swift 同顺序 Double 运算及 nearest-away-from-zero 舍入。例如 1/8 为 13%，5/8 为 63%，不能截断或 bankers rounding。

### historical floor

对应 `NoonmarkEngine.swift:4004–4020`。

floor 是**更早日期**，或**同日较小 continuationSeq**的 history-forming trace 各自 raw percent 的最大值。最终 `Percent = max(raw.Percent, floor)`，而 weights 保留目标 raw 的 weights，所以最终 percent 可高于 completedWeight/totalWeight。

不要错把同日规则套进 raw：raw 只按 date 截断。同日较高 continuationSeq 的 child 可以影响较低 seq 的 raw；它的 raw 本身却不能进入低 seq 的 floor。同 date、同 seq 的多个 trace 也不会互相成为 floor。future records 不改变较早日期的 raw 或 floor。

取消 trace 不贡献 floor，但如果调用者显式查询这个 ID，仍按源返回其自己的 manual 值或可见 chain 历史权重及 floor；本 API 不负责隐藏 trace row。

### 手动编辑与父完成能力

对应 `NoonmarkEngine.swift:2840–2864, 3937–3948, 1979–1987, 3684–3708`，`Models.swift:42–58`。

manual 可编辑必须同时满足未锁定、目标就是 today、父 pending、chain 的所有 history-forming trace 都没有可呈现 child。这里没有 date cutoff：future child 也会阻挡 manual 编辑，即使目标显示 mode 仍为 manual。definition.plannedSubtasks 不参加该判断。不存在实际 child 的纯计划不会自行变成 weighted 模式。

父完成能力按顺序检查 locked day、historical day、future day，然后父状态：pending 看**当前 trace**的 child，completed 允许 undo，其他状态为 unsupportedStatus。历史 chain 里有 child，不等于当前父不能完成。100% 也不会自动完成父 trace；最终仍由原有 command 决定并重新验证。

## 一次构造与复杂度

新增投影算法按 chain 排序 trace，再按 date／continuationSeq 分组扫描。一个 date 组先吸收全部可见 child 的 lineage 最大 weight 与 any-completed；然后逐 seq 组投影，整组写完才推进 raw 最大 floor。源代码对 records 的 sort 后执行 max／contains，二者与排列无关，因此无需复制每个 lineage 的排序。

设 T 为 traces，S 为 child records，N 为输入图总事实数。已有 validator 成本不变；新增投影扫描成本为 `O(T log T + S)`，整体构造另有 `O(N)` 的深复制，临时空间 `O(N + T + S)`，保留的视图空间线性。四种 row 查询为 O(1)，无全图递归、无逐 row 重新验证或扫描历史。

## 测试与源测试映射

- `TestProgressOfficialWeightedSubtaskExample` 对应 `testWeightedSubtaskProgressUsesDifficultyAndBlocksManualProgress`。
- `TestProgressOfficialManualContinuationFloor` 对应 `testManualProgressCarriesForwardAndCannotRegressAfterContinuation`；本轮只投影，不伪造一个新的 setter。
- `TestProgressExistingCommandsDriveCompletionAndCancellation`、`TestProgressAbandonedCountsInWeightButNotCompletionBlocker` 对应 `testParentTraceCannotCompleteWithOpenSubtasksAndShowsPartialProgress`、`testCompletionCapabilityUsesCurrentTraceStateAndSubtaskProgress`。
- `TestProgressCompletionCapabilityGuardsAndStatuses` 对应 `testCompletionCapabilityRejectsWrongDateLockedDayAndUnsupportedStatus`，扩展覆盖全部 8 个父状态 ×6 个日期／锁定组合。
- `TestProgressHistoricalChildrenDoNotBlockCurrentParent` 对应 `testCurrentTraceCanCompleteWhenOnlyHistoricalChainTraceHasSubtasks`。
- `TestProgressCancelledTraceAndChildHaveDifferentProjectionRules`、命令取消流程对应 `SnapshotUndoCancellationTests.testUndoingAddedSubtaskRetainsHiddenSubtaskFact` 的投影预期。这里没有实现或声称运行 snapshot undo command。
- 额外覆盖最大难度 3→2→1、后来增加已完成 lineage 的 weight、早期高 floor、同日相同／不同 seq、future、取消 lineage 不抹掉早期权重、不同 chain 隔离、不同 definition 同 lineage、abandoned chain 历史、定义仅 planned child、舍入边界、不存在 ID、nil/零值投影、错误图不 panic、不修改输入、并发读。
- `sourceShapeProgress` 是测试内按上游 group/sort/max/contains/scan-floor 形状独立写的慢版 Go，交叉检查 sweep。120 组合成历史逐 trace 对账；这不是 Swift oracle。
- `progressYearGraph`：24 chains ×365 traces ×4 children，即 8,760 traces、35,040 child records，带严格有效的双 lineage continuation 链、completed 与 abandoned 记录。三轮对所有实体切片洗牌，四类查询对全体 trace 逐一比较，并抽样与慢版源形状算法对账。

## 实际验证与性能

运行环境：Go 1.27.3，Linux/amd64，AMD EPYC 9V74，GOWORK=off、GOTOOLCHAIN=local、GOPATH=/tmp/noonmark-gopath、GOCACHE=/workspace/shared/noonmark-go-build-cache-20261105。所有日志、coverage、fuzz cache 和 test binary 外置于 `/workspace/shared/noonmark-subtask-progress-build`。

实际命令与结果见同目录 `verification-meta.txt`、`race-all.jsonl`、`vet-all.log`、`core-coverage.log`、`core-coverage-functions.txt`、`progress-benchmarks.txt`、`progress-guided-fuzz.log` 及交付 `manifest.json`。不会把 focused test 当作全项目通过，也没有运行 UI／Service／持久化／跨平台门禁。

基准是当前容器的三次 1 秒样本，包含构造前验证和深复制，不是性能承诺：

- 1 chain ×365 traces 构造：2.85–2.99 ms，约 2.95 MB 分配。
- 8 chains ×365 traces 构造：20.6–23.1 ms，约 21.75 MB 分配。
- 24 chains ×365 traces 构造：74.8–86.0 ms，约 70.39 MB 分配。
- 8,760 rows 的四类查询一轮：2.60–2.67 ms，0 B／0 allocs。
- 单个 TraceProgress 查询：12.81–12.97 ns，0 B／0 allocs。

最终重跑结果：`go test -race -count=1 -json ./...` 的 core／textcanon 两个 package 均通过，357 个 test/subtest pass entries（112 个顶层）；本轮 31 个 TestProgress 顶层测试、含分支共 94 个 pass entries，另含 16 个 fuzz seed。`go vet ./...` 通过。core statement coverage 92.3%，`progress.go` 的 9 个函数均为 100.0%。这些数字不含 UI／Service 门禁。

正式 guided fuzz 在 8 秒内完成 204,185 executions，16 个初始 seed，新增 3 个 interesting inputs，共 19 个，PASS。

最初外置 fuzz binary 未带 coverage guidance，虽 8 秒跑了 589,049 次并通过，仍仅作补充。随后以 `go test -c -fuzz '^FuzzProgressProjectionNoPanic$'` 重建带引导 binary，在证据目录运行并外置 corpus；正式 fuzz 结果以 `progress-guided-fuzz.log` 为准。

## Owned 文件与后续整合注意事项

只交付以下 7 个新文件，SHA-256 另见外置 `owned.sha256`／`manifest.json`：

1. `domain-core/core/progress.go`
2. `domain-core/core/progress_fixture_test.go`
3. `domain-core/core/progress_test.go`
4. `domain-core/core/progress_equivalence_test.go`
5. `domain-core/core/progress_commands_test.go`
6. `domain-core/core/progress_benchmark_test.go`
7. `docs/tasks/subtask-progress-core/README.md`

本轮没有改变任何 command 或 validator。整合时应复制这 7 个文件并重新跑相关 core 门禁；父能力是快照投影，不应代替 mutation 时的原有校验。UI 若接入，必须同时保留未覆盖能力提示和取消事实隐藏边界，不能用简单 completed/count 代替 TraceProgress。Service 原子事务、持久化重启、完整 Swift 差分 oracle 与其他 OS 实机都仍待另行授权和验收。

## 独立复审与正式整合

独立审核已验证全部7份冻结文件，并完整复跑原候选race/vet；新增5份顶层测试后362个test/subtest entries、117个顶层全部通过。包括实际Double的23/40→57%、同日seq/raw/floor差异、1000有效图谓词差分、深层输入修改并发与四查询零分配。此次整合保留全部5项独立测试作为永久回归，未改原31项测试断言。Swift oracle与运行UI仍未验收。正式整合 scripts/check 已 exit 0，应用main race79.322s、Service22.940s、core9.939s、native ui2.528s/root1.052s；四模块race/vet及51项依赖门禁通过。六目标cross-build、Linux隔离CLI create/reopen/拒绝覆盖也实际通过，证据外置 integrated-{check,cross,entry}.log。此结果仅是本地整合，尚未推送或运行新head CI；投影UI接线仍另行开发。
