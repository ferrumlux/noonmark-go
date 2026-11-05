# Noonmark Go 领域内核第一批

这是可以单独合并的第一批 Go 代码，不是完整 Noonmark 替代品。源自 `quboliu/noonmark` commit `43e6eefaae3a04be89cab3e234d470111afd80ef`，保留 AGPL-3.0 LICENSE 与 NOTICE。本目录不依赖 MyGo、第三方 Go 包、用户资料或网络。

## 已实现

- 强类型 Day、TaskChain、TaskDefinition、DayTrace、TaskNoteEntry、PlannedSubtask、Subtask 及关联 ID。保留八种 trace 状态、历史链接、附言墓碑、精确时钟、取消 witness；没有简化成单一 Task。
- `Instant` 保存 Swift Date 的 IEEE754 UInt64 位模式，拒绝 NaN/Infinity、精确 JSON 大整数往返；`NextMutationInstant` 按 nextUp 严格前进。
- `LocalDate` 严格 Gregorian 日期，1..9999 年，UTC noon 日期运算；不把自然日当24小时或任务的 UTC timestamp。
- `ValidateTaskGraph` 检查本批实体的身份、时钟、状态事实、父引用、定义版本/当前项/后继环、计划子任务、活动trace唯一性、priority槽、延期/延续/变更链接、子任务lineage/顺序/因果环、取消身份与附言墓碑。
- `TaskKernel` 提供串行、深复制、验证后发布的基础命令：创建入池、排期、完成、撤销今日完成、日结。失败不会留下半个状态；Snapshot返回深复制。
- `DayTodo` 正确区分用户可见状态、movement事实与内部取消，并过滤同日新事实已替代的旧延期源；默认置顶/待办/终态排序。
- 本次有34个顶层测试/模糊测试入口，91个通过事件（包括子测试与seed），覆盖率90.5%。Linux amd64上的race检测、vet与5秒fuzz通过。

## 明确边界

1. `TaskGraph` 是完整 `NoonmarkSnapshot` 的一个有界切片，不含分类历史、重复计划父项、飞光、preferences、sync ledger、AI sidecar。它**没有v7导入/导出codec**。
2. `ValidateTaskGraph` 不能代替原作完整snapshot验证。带`CycleMembership`的模型可以检查本地字段，但不会检查父series；`NewTaskKernel`因此明确拒绝重复实例，避免没有父计划还擅自修改。
3. `SettleTaskDays`只生成任务侧日结事实。整合到完整引擎时，必须在同一事务捕获分类快照，再持久化/发布。此版本不得直接接上真实生产数据库。
4. 任务标题、附言的trim暂用Go Unicode White_Space；未跑Foundation字符串跨引擎golden。ID要求内部大写规范UUID，未来Swift wire decoder应先解析、规范化再构造此图。不会冒称全部Swift Codable/Unicode兼容。
5. 未实现延期/延续/变更等命令、完整Undo/Redo、分类、repeat engine、飞光、数据包、SQLite、同步、AI、系统集成与UI。已有这些事实的图可用于本批拓扑测试，但不是上述功能已实现。
6. 未执行Swift对照引擎、真实Mac/Windows/ARM64运行或任何生产数据验证。六目标测试二进制的交叉编译只证明本包可构建，不证明各平台运行，更不证明MyGo窗口/IME支持。
7. 调用方必须向完整mutation clock提供全部模块的持久时钟。`Instant`不可按底层uint64大小排序，应使用Before/After/Equal；DateBits标量JSON支持不等于Foundation canonical JSON数据包。

## 接口与合并方式

模块：`github.com/ferrumlux/noonmark/ports/mygo/domain`，包：`core`。Go最低声明1.23；当前验证工具链为Go1.27.3 linux/amd64。

可以整体放到`ports/mygo/domain/`，主模块添加本地replace；也可把`core/`整体迁至主模块的领域目录并调整import路径。不要复制`.cache/`、`coverage.out`或跨平台测试二进制。保留LICENSE/NOTICE，后续提交按英文commit规范。

基础使用顺序：

1. 在合成fixture上构造`TaskGraph`，调用`NewTaskKernel`。
2. `CreatePoolTask`显式提供chain/definition ID与时间；按需要提供description与初始note。`NewIdentity`可创建规范UUID，测试重放应使用固定ID。
3. `ScheduleFromPool`提供trace ID、日期、today、时间；新日期还需day ID；每个planned subtask必须提供对应新subtask ID映射。
4. `MarkCompleted`/`UndoCompleted`受日期、锁定状态、开放子任务与内容时钟约束。
5. `SettleTaskDays`仅作任务内核阶段验证，未来由完整事务协调器绑定分类模块。
6. 调用`Snapshot`或`DayTodo`获取深复制，UI不得直接修改内部图。

`ValidationError`有可检查的Issue code；`Error()`仅返回违规数量。原始Issue可能包含不可信导入ID，不允许直接送入正式诊断日志。

## 复跑

```bash
GO=/absolute/path/to/go NOONMARK_DOMAIN_BUILD_ROOT=/tmp/noonmark-domain-build ./scripts/check
```

`check`执行只读格式检查、`go vet ./...`、`go test -race -count=1 -cover ./...`。缓存外置到指定BUILD目录。

模糊测试：

```bash
go test -run='^$' -fuzz=FuzzTaskGraphValidationDoesNotPanic -fuzztime=5s -parallel=2 ./core
```

全部测试使用人工合成数据，无真实账号、Keychain、iCloud或用户任务。测试输出在`evidence/`，机器可读验证摘要在`evidence/verification.json`。

## 上游来源与测试对照

| Go文件/能力 | 上游源码 | 对应上游测试主题 |
|---|---|---|
| models.go | Sources/NoonmarkCore/Models.swift、Identifiers.swift | NoonmarkEngineTests、NoonmarkSnapshotDefinitionTopologyTests |
| time.go | MutationClock.swift、LocalDate.swift；Sources/NoonmarkStorage/ExactDateJSONCoding.swift；Sources/NoonmarkDayContext/GregorianCivilCalendar.swift | MutationClockBoundaryTests、DataPackageTests的submillisecond dates、GregorianCivilCalendarTests |
| validation.go定义 | TaskDefinitionValidator.swift、NoonmarkSnapshot.swift | duplicate sequence/current count、restoration witness、planned subtasks、broken parent references |
| validation.go轨迹 | TrajectoryTopologyValidator.swift、NoonmarkSnapshot.swift | duplicate active trace、deferral vs continuation、changed link、cancelled draft、subtask lineage/topology |
| kernel.go | NoonmarkEngine.swift 的 createPoolTask、scheduleFromPool、markCompleted、undoCompleted、settleDays、getDayTodo、sorted、isInTaskPool | Pool→schedule→complete→undo→settle、immutable history、open subtasks、atomic failure、superseded deferral |
| clone.go | 此Go内核自身隔离实现 | SnapshotIsDeeplyDetached、ConcurrentCreationAndReads、rejected mutation不改变状态 |

这些是源码驱动的移植测试，**不是已经运行了原Swift测试**。完整跨引擎差分与官方一年demo golden应继续按`../migration-audit/NOONMARK-MYGO-MIGRATION.md`的P0推进。

## 本批发现并修复的回归

`DCORE-001`：首次Go DayTodo投影只按状态过滤，导致“延期→目标回池→源日重新排期”的旧deferred源仍占一行。根因是遗漏上游`getDayTodo`的同日、同chain最新createdAt过滤。测试`TestDayTodoFiltersSupersededDeferralAndKeepsMovementFacts`先失败，再加入同chain最新历史事实筛选后通过；源trace仍留在完整图，不做删除。该测试进入`./scripts/check`。此项是新Go移植过程中的问题，不归因为上游故障；尚无提交SHA，故不冒称已发行修复。
