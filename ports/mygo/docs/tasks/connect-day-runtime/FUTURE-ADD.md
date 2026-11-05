# 选定日期快速新增：任务服务

## 需求与现状

为 Day 快速新增提供 `Service.AddOnDate(title string, date core.LocalDate) error`，允许今天与未来未锁定日期。此改动只扩展真实 development 服务命令，不修改 UI、领域模块或持久化格式，也不开放任何未支持 supplement。

上游源码依据（只读参考，不运行 Swift 或导入资料）：

- `App/NoonmarkMacApp/DayTodoPage.swift:338`：`DayQuickAdd` 按未来／今天展示输入提示，提交调用 `store.addQuickTask()`。
- `App/NoonmarkMacApp/NoonmarkStore+TaskMutations.swift:517` 起：选定日期快速新增在同一个 candidate mutation 内执行 `createPoolTask` 和 `scheduleFromPool`；今日快捷入口使用 mutation moment 的 today。
- `Sources/NoonmarkCore/NoonmarkEngine.swift:1424` 起：排期拒绝过去日期与锁定日期。

当前 Go 服务已经具备 `AddPool`、`AddToday`、`Schedule`，以及事务内完整 task + 空 catalog classification history 审计。新增命令复用这些边界，不绕过审计，也不声称涵盖上游快速新增的分类、重复计划或 origin description 等完整能力。

## 方案与实施

三个新增入口共用一个私有 `add`：任务池不排期；今日快捷在 `transact` 持锁期间解析当前 today；指定日期入口保留调用者明确选择的日期，再由 `ScheduleFromPool` 对照持锁的 today 验证。不能在锁外先读 today 再传入指定日期入口，否则跨日等待可能拿着旧日期新增。

同一候选 kernel 内先建立 chain／definition，再建立 trace／day；只有两条领域命令、完整聚合审计、规范化、磁盘提交与回读确认全部成功后，才发布服务快照。排期拒绝会丢弃整个候选，不遗留池任务或孤儿 chain。每次成功新增只有一个磁盘 revision，不额外拆分创建和排期事务。

沿用现有故障契约：

- reservation 之前失败：内存与磁盘均不发布，可重试。
- reservation 后但提交标记前失败：保留不完整 revision，服务阻断续写，重开 fail-closed，不自动修补。
- 已持久化但确认返回前失败：调用仍报错且原服务不发布；重开可读到完整已提交任务，禁止原服务盲目重试造成重复。
- 两个服务竞争同一 revision：只有一方提交成功；另一方返回冲突且不发布候选。

## 验证范围

`internal/taskapp/future_test.go` 使用本轮新建的合成 development 会话，覆盖：

1. 未来快速新增 → 今天完成被拒绝 → `AdvanceTo` 到目标日 → 完成 → 下一日结算 → 重开；逐字节比较完整 aggregate，检查空分类审计与精确 revision 数。
2. 过去、零值、非法日历日期、空标题、锁定今天及锁定未来日期全部拒绝；无 orphan、audit 变化或 revision reservation。
3. `AddPool`、`AddToday` 及 `AddOnDate(today)` 保持原有语义。
4. `before-reserve`、`after-reserve`、`before-commit`、`after-persist` 故障下不提前发布；安全重试、阻断不确定续写及重开结果符合现有存储契约。
5. 双 handle 竞争无丢失更新；跨日结算占锁时今日快速新增等待锁后才解析 today。

验证命令（在 `ports/mygo` 工作目录执行，Go 使用工作区安装版本）：

```sh
GOCACHE=/tmp/noonmark-day-build/cache \
GOTMPDIR=/tmp/noonmark-day-build/tmp \
TMPDIR=/tmp/noonmark-day-build/tmp GOMAXPROCS=2 \
/workspace/scratch/586e77bd59a6/noonmark-mygo-work/go/bin/go test -p=1 -race ./internal/taskapp
```

只验证 Go 隔离服务；未运行 production、iCloud、Swift oracle 或非 Linux 实机。此文档不构成 UI 路径已接通或完整上游对等的验收声明。

本轮结果（2026-11-05）：完整 `taskapp` race 测试通过（5.140s）；新增日期／跨日测试另以 `-count=10 -run 'TestAdd(OnDate|TodayResolves)'` 重复通过（15.594s）；`git diff --check` 通过。未 commit 或 push。
