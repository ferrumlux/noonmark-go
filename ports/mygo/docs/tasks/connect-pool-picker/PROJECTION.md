# 任务池选择器纯投影

## 目标与边界

`taskview.ProjectPool(core.TaskGraph) ([]PoolRow, error)` 返回独立值行，字段为 `ChainID`、`DefinitionID`、`Title` 和 `CreatedAt`（当前 definition 的创建时刻）。不读 clock、不运行命令、不改图、不具持久化权限。运行资料只来自合成 development 测试，不读取 production。

先执行正式 `core.ValidateTaskGraph`，包括不显示的链及 subtask；任一错误返回 nil 行及可解包的校验错误，不产生部分结果。此校验只涵盖 bounded graph，不能替代完整 snapshot、分类和 recurring parent 校验，也不扩张 Service 支持范围。

## 上游对照

只读上游 `Sources/NoonmarkCore/NoonmarkEngine.swift`：

- `taskPool`（1598 起）：按当前 definition 的 `createdAt` 升序，不是链的创建时间。
- `isInTaskPool`、`ChainTraceFacts`（3829–3918）：仅 active、非 recurring 链；任何 pending trace 都排除；无 trace 可入；否则 latest 必须是 returnedToPool 或 cancelledDraft。
- latest 先比 `contentUpdatedAt`，再比 `createdAt`，最后比 trace ID，各取较大者。取消草稿也参与 latest，不能因其不形成日历史而忽略。
- 当前 definition 用 `SupersededAt == nil`；保留 restoration witness 的 `SupersededByDefinitionID` 不等于已被取代。

实际阅读并对照的上游测试：

- `NoonmarkEngineTests.testSchedulingFromPoolCreatesDayTraceAndRemovesFromTaskPool`：新任务入池，排期后移出。Go 对应测试用正式领域命令合成，并补完成后的排除。
- `testTaskPoolRemovalHidesUnscheduledFactsAndPreservesReturnedHistory`：abandoned 链即使无 trace 或曾回池亦不显示。
- `testDeletingNewCurrentDayTaskLeavesOnlyAnInternalCancellationFact`：cancelledDraft 本身不足以入池，abandoned 链仍排除。
- `testDeferredFutureTargetCanReturnToPoolWithoutRewritingSourceHistory`：旧 deferred 和新 cancelledDraft 共存时仍可回池；Go 测试保留因果链接。
- `SnapshotUndoCancellationTests` 中撤销未来排期场景（约 90–127）：active 链保留 cancelledDraft 后回池。
- `TaskCycleSeriesTests.testRecurringOccurrencesStayOutOfGenericLifecyclePools`：recurring 不进入通用池。

## 明确差异与验证

Swift 仅规定 definition 创建时刻比较，同刻没有规定跨 dictionary 次序。Go 在同刻用 `ChainID` 升序，明确提供跨平台确定性，不宣称复现 Swift 的未规定 tie 次序。时间比较使用 `Instant.Before/After/Equal`，不按保存 IEEE-754 bits 的整数值排序。

Go 测试额外覆盖旧 definition 与当前标题/排序、latest 三层比较、pending 优先排除、输入顺序变化、非法隐藏图拒绝，以及输入输出互不别名。合成边界图不是 UI 假资料；运行界面必须从正式 snapshot 调用投影。

验证命令：`go test -p=1 -race ./internal/taskview`、`go vet ./internal/taskview`，Go 1.27.3、外置 `/tmp/noonmark-gopath` 与 `/tmp/noonmark-gocache`。这属于 Linux Go 验证及 Swift 源码/测试语义对照；未运行 Swift oracle，不宣称完整跨引擎对等或非 Linux 实机通过。
