# 普通任务完成层次及未完成池

task-id: project-task-history

## 需求与现状

A 级。基线 Go `8d15eacbc3a84ebc00571fafc618de77c2faca90`，Swift `43e6eefaae3a04be89cab3e234d470111afd80ef`。本批只新增纯投影，不修改 runtime、导航、持久化、validator、codec 或分类／重复任务边界。已读上游 AGENTS、CONTEXT、本端 AGENTS/DESIGN。普通完成页面目前不能表达父任务仍待完成而子任务已有完成记录的情况；本批先提供 API，尚不修复 UI。

## 设计（代码前）

新增 internal/taskview 的 HistoryProjection。构造时验证整个 TaskGraph 并深拷贝；查询返回脱离输入及其他查询结果的值。CompletedTaskHierarchies 按链聚合完成父轨迹及完成子记录，以 lineage 去重子项；父标题来自选中的完成轨迹定义，只有子项时采用当前定义。保留完整历史轨迹及子项轨迹，不压缩成最后一条。UnfinishedPool 选 active/abandoned 普通链，有 unfinished/abandoned 历史且没有 completed 父轨迹；使用当前定义，覆盖展示用 chain.updatedAt 为候选 createdAt 最大值，并保留所有日期化明细、活跃轨迹和回池状态及动作计划。

排序遵循 Swift 明确的键。不增加源代码没有的 UUID 末键。只在原版明确使用 UUID 的排序中使用 UUID。Swift 未指定相等键顺序的地方保留 comparator 的相等语义及稳定排序；Dictionary 枚举相等项次序无法保证，因此只对有源明确顺序的资料验证 permutation invariance。localizedStandardCompare 的 locale 行为无法在本机证明，因此构造器必须传入纯 TitleCompare 比较器；不暗中以 byte、lowercase 或近似 collator 冒充 Swift。生产接 UI 前需要注入经验证的平台比较器。测试用比较器只验证投影的调用及后续键，不作 Swift 排序 oracle。

## 源码依据与边界

- NoonmarkEngine.swift:1750–1977，unfinishedPool、completedPoolItems、completedSubtaskRecords、completedTaskHierarchies、latestCompletedChildren。
- NoonmarkEngine.swift:2271–2286、4020–4121，carryoverKind、completedTrajectory、子轨迹及稳定日期去重。
- Models.swift:1245–1359，未完成动作与 latestCompletionAt；FormsDayHistory 包含 returnedToPool/changed，排除 cancelledDraft。子项排除 cancelledDraft，其余终态保留在轨迹。
- 本内核只有 active/abandoned 链态；“完成链”是存在 completed 轨迹，不发明 ChainCompleted。
- 本内核缺少普通链 Continue、Defer、Abandon/Reactivate 命令。真实命令覆盖 Create、Schedule、EditSubtask、Complete/Undo、Settle、Return/reschedule、Rename；缺命令的拓扑只能作明确标注的 validator 合法结构测试，不能宣称命令或端到端能力。
- 普通投影忽略已验证图中的 cycleMembership 链，不解除 runtime 对重复任务的拒绝。TaskGraph 不携带 catalog，分类及完整 snapshot supplement 仍由已有入口 fail-closed；本 API 不验证被省略的 supplement。

## 验证与回滚

通过真实领域命令测试跨版本、同日回池／重新排期、跨日结算、完成子项／未完成父项、删除 tombstone、所有子状态、输入输出隔离、相等时钟语义、有确定源顺序的排列不变性及非法图拒绝。另写缺命令明确的结构测试。源码和 UI 运行无变化，回滚仅删除本批新文件。所有新测试证据与缓存放显式外置 NOONMARK_MYGO_BUILD_ROOT；本机未运行 Swift oracle 或原生 UI，不宣称二者通过。

## API 交接与后续 UI 规格

`NewHistoryProjection(graph, compare)` 要求显式纯 `TitleCompare`；返回只持有私有深拷贝的投影。`CompletedTaskHierarchies()` 返回每链的 Chain、Definition、可空 ParentCompletion、去重 CompletedChildren 与 LatestCompletionAt。ParentCompletion.Trajectory 保留日期摘要、全部 formsDayHistory 轨迹与按 lineage 分组的全部可呈现子轨迹。`UnfinishedPool()` 返回当前 Definition、所有 UnfinishedTraces、可空 ActiveTrace、IsInTaskPool；`LatestUnfinishedTrace()`、`IsAbandoned()`、`ContinuationCount()`、`IsContinuedPending()` 和 `ActionPlan()` 是源语义提示，并非命令可执行许可。所有返回值均脱离输入及下次查询；调用者可以编辑自己的副本，不能借投影改变领域图。UI 仍须由 command service 再验证当下状态与命令支持，不能执行尚未移植的动作。

排序比较器目前没有源 locale 实现，不得接入 UI 后声称对等；未实现任何 UI 变更。Swift 同日子轨迹按 position 而不是 createdAt 优先，因此回池后 position 重排可能让较晚生成的完成子记录排在原同日 pending 记录前；测试按真实命令及此源排序断言，不改成“时间直觉”。

供下一批 UI 复刻：

- CompletedPoolPage.swift:49–66 使用当前分类组织 hierarchy，并以 LatestCompletionAt 作 presentation time；分类不来自历史 definition。107–138 的列表零纵向 spacing，顶部 12、底部 20，水平 pageHorizontalPadding；默认／偏好组织属于独立 presentation projector，不能由本投影擅自强制分组。
- 149–180 初始子项展开，collapsedCompletedHierarchyIDs 是独立视图状态；本批不持久化折叠状态。
- 234–350 父行 VStack spacing 8、HStack spacing 10、标题 13 medium、最小高 52、水平 padding 12、竖直 padding 8；只有 completed parent 才显示完成字形、浅字与删除线。轨迹至少达到 MacUICompletedPoolRowLayout 门槛才显示，左缩进 28。
- 363–431 子行 HStack spacing 10、首个空白宽 28；已完成父项下子项为安静短横线（18×18），未完成父项下子项保留 checked 字形。子标题 12.5 medium、最小高 40、水平 padding 12、竖直 padding 2；时间来自该子记录的 parent date 和 child completedAt，非整链最新日期。
- WorkspaceUIPrimitives.swift:691–769 的 listRowSurface 默认圆角 7；分隔线高 1，普通 alpha 0.62，选中隐藏；选中色填充 alpha 0.08、边框 0.22；父分隔缩进 40、子分隔缩进 70。不是每个 hierarchy 另加圆角卡片。
- UnfinishedPoolPage.swift:211–280 使用历史条数、continuation count 与最近日期的轻量 metadata，活跃安排／回池是关系注记而非覆盖旧状态；展开明细 spacing 6、左缩进 30，行水平 padding 12、竖直 10，分隔缩进 40。

## 本轮验证记录

新增 12 个 History 测试：6 个命令／边界测试，4 个明确标注的结构语义测试，另有 metadata 隔离与显式比较器测试。真实命令已证实完成子项独立出现、parent completion 显式发生、完成 undo、子项删除墓碑、同日回池保留旧完成子项、重新排期携带 lineage、未来回池隐藏取消草稿、跨日结算进入未完成池、版本化标题。结构测试补充 Continue/Defer 链、已废弃链、重复完成 lineage 去重、历史／当前 definition 分离与 source equal-clock 语义，不伪称这些是当前命令路径。

当前 RenameTaskTitle 在已完成轨迹之后返回 invalid transition；没有为测试放宽命令或捏造可执行流程。跨版本的完成旧定义选择由真实回池历史及明确结构测试分别覆盖。Swift 对账、locale 比较器及 UI 接通仍未完成。

## 查询成本与调用边界

构造器执行整个图 validation 并 clone；一次查询再次 clone 图、建 chain/definition/trace/subtask index，并排序结果及各链的历史。空间成本随整个图及返回历史增长。完成投影除建 index 与各历史组排序外，appendHistoryDate 对每个 continuation 日期作线性去重；每条父轨迹或子 lineage 的 k 个日期最坏需 O(k²)，因此不能把完成查询概括为仅 O(图大小 + 排序)。源 uniqueDates 使用 Set；本批保留已冻结实现，待真实长链成本证据及回归后再优化。未完成投影约为 O(图大小 + 候选组排序)。TitleCompare 成本由注入方决定。API 是不可变读取边界，不是每帧低成本 selector。

后续 UI 应在 command service 确认发布新 graph 时构造及缓存该版本的投影结果；绘制、滚动、hover 与折叠切换只读已缓存结果，不每帧构造、不为每行重复查询、不为导航徽标调用完整层次投影。不要以去掉验证或返回底层可变指针优化。此批不扩范围实现持久化缓存或 UI 缓存。

2026-11-05，Linux amd64、AMD EPYC 9V74、Go 1.27.3、GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB，一次 `BenchmarkHistoryYear`（1s/子项，count=1，无 race）测量：

- 通过真实 CreatePoolTask、ScheduleFromPool、Add/CompleteSubtask、MarkCompleted、SettleTaskDays 合成 365 天、365 链、365 轨迹、365 子项；结果 183 完成层次、182 未完成项。fixture 准备不计入子项计时；它不是完整 interactive-demo 合约，也不覆盖单链长 continuation 历史；不能据此证明长链性能。
- 构造验证快照：1,304,671 ns/op，988,706 B/op，4,099 allocs/op。
- 完成层次查询：1,617,161 ns/op，1,527,872 B/op，4,310 allocs/op。
- 未完成池查询：1,442,647 ns/op，1,263,880 B/op，3,026 allocs/op。

这里只记录单次当前环境证据，不设未经用户或实机证明的性能阈值，不宣称 UI 帧率保证。永久 benchmark 与测试一同保留。另纳入独立审查的 6 个测试，覆盖 completion moment 与 date 差异、源 equal-clock lineage UUID、选中 completion 后的完整轨迹、最早子 title/date、相等日期稳定排序、动作优先级、latest content clock 及全图验证。

## 最终冻结与验收范围

- 最终四份 Go 文件已冻结；作者 12 个测试及独立审查 6 个测试纳入默认 taskview 测试集。最终 `go test -race -count=25 ./internal/taskview` 与 `go vet ./internal/taskview` 通过。
- 本轮 `scripts/check` exit 0，覆盖 app、三个 domain 模块的 race/vet、Unicode 门禁及 MyGo dependency/native 门禁。但该聚合在加入最后 metadata 方法、两个作者测试、独立测试、benchmark 和最终文档之前已启动，只记作**预冻结聚合结果**，不能外推为最终五文件 exact gate。正式整合后仍须由整合者对冻结版本执行完整门禁。
- 本批尚未接 UI，不能宣称修复当前已完成页面；未运行 Swift oracle、原生实机 UI 或 source locale 对账；未更改任何正式仓库、远端或发行配置。
- 交接仅新增 `internal/taskview/history.go`、`history_test.go`、`history_independent_review_test.go`、`history_benchmark_test.go` 与本 README。Task ID 为 project-task-history。

## 正式冻结整合验证

2026-11-05 UTC，在正式副本0e600aa基线仅加入五份冻结文件。独立审查通过的实现、12项作者测试、6项独立测试及benchmark字节保持不变，未改任何运行UI、store、codec或validator。正式scripts/check在全部文件已就位后启动，期间没有改源，最终exit0：root race190.240秒、taskview1.052秒，四模块race／vet、Unicode16完整离线门禁、51项dependency与native UI/popup均通过。新增Swift比较工具的自动独立checkout28项也在本轮聚合中通过。

六目标cross-build和Linux实际二进制的隔离create/reopen/day+pool渲染、拒绝覆盖与缺少身份拒绝均exit0。外置证据为noonmark-history-build-20261105/evidence/integrated-check.log、integrated-cross.log、integrated-cli.log。以上是最终正式冻结代码的本地聚合，不引用作者预冻结聚合替代它。只有本文在测试结束后补充记录。

尚未接通Completed／Unfinished UI层级、来源locale比较器及缺失历史命令，长链日期去重性能仍待专门验证；六编译不是六平台实机，未执行Swift对账。远端待批写入未重试，无新GitHub CI或发行。
