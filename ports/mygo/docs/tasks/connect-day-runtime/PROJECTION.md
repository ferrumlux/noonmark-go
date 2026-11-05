# Day Todo 纯只读投影方案

任务：`connect-day-runtime`。2026-11-05，先记录方案再实现。

## 范围与证据

只新增 `internal/taskview/day.go`、测试与本文；不改 Calendar、UI、领域内核、Service 或存储。仅支持原版默认 flat/time/ascending，不提供分类、标签、grouped、title 排序或写操作。依据上游 `NoonmarkStore+PresentationUndo.swift:337–356,429–445`、`TaskCollectionPresentation.swift:157–189,223–251`、`ReviewRail.swift:17–82` 与本轮只读研究。

## 接口与日期

- `ProjectDateStrip(today, selected) (DateStrip, error)` 返回固定 14 槽 `Cells` 和 `SelectedIndex`。每槽为 `Date *core.LocalDate`、`IsToday`、`IsSelected`、`Pending`。纯日期入口的 Pending 为零，完整 ProjectDay 填真实计数。
- `ProjectDay(graph, selected, today) (DayPage, error)` 返回所选日 `Rows []DayRow`、`Stats DayStats`、日期条与 `Review *DayReview`、history/future 标志。DayRow 嵌入 CalendarRow，仅增加真实 `Pinned` 和复制后的 `PinOrder`。
- 日期采用 Gregorian ordinal，0001-01-01 为 1，不使用跨年 time.Duration。固定锚是 today ordinal−6，页号是 floor((selected−anchor)/14)。两端越界保留 nil 槽，不夹成重复日期；selected 总是存在且可定位。趋势同样保留越界 nil 日期。

## 资格、排序与统计

先完整 ValidateTaskGraph，再深复制；只读克隆上清除 recurring command guard，但保留其可见轨迹。Day 与 Calendar 的范围不同：上游 `NoonmarkStore.swift:811–814` 直接调用 `NoonmarkEngine.getDayTodo`，后者无 recurring 排除条件，`DayTodoPage.swift:663–727` 还展示周期标记。不得将 Calendar 的 recurring 排除套用到 Day。克隆仅用于读操作，不扩张普通 Service 或领域命令接收范围。所有列表、pending 点、统计与趋势均从同一 kernel.DayTodo 资格取事实，按日期缓存查询。

真实 pinned pending 队列先出，pinOrder/ID 升序；其余 pending 优先，再 createdAt/ID 升序。Instant 使用领域 Before/Equal，不能比较 IEEE bits，也不能按 priority 假冒 time。标题只按 trace.DefinitionID；terminal 携带 pin 是领域无效输入，严格拒绝。

DayStats 包含 Total、Completed、Pending、Unfinished、Deferred、Abandoned、CompletionRate；完成率空日为零，否则四舍五入百分比。Review 仅在 selected≤today 存在，Trend 为 selected−6…selected 七槽；无任务 Ratio=nil，有任务零完成 Ratio 指向 0。TomorrowTotal 仅 selected=today 时计算真实明日总量，日期超界不查询。未来仍有真实列表与统计，但 Review=nil。

## 验证门禁

表驱动覆盖固定锚正负分页、两端、跨世纪/千年、闰日、invalid date；排序覆盖 priority 干扰、pin 相同序、相同 createdAt、负 Instant、pending 优先；统计覆盖空日、1/3、2/3、1/24、全完成、unfinished、future、trend nil/0；覆盖隐藏状态、superseded deferred、历史标题、recurring、非法图及输入/输出隔离。测试缓存和日志放 `/tmp/noonmark-day-build`，低并发运行 focused test、race 与 vet。这里只证明 Go 只读投影，不声称 Swift oracle 或 UI 实机验收。

## 本次结果

已完成上述纯投影，默认 API 与方案一致。2026-11-05 实际通过：

- `go test -p 1 ./internal/taskview`（含既有 Calendar 回归）。
- `go test -race -count=1 -p 1 ./internal/taskview`。
- `go vet -p 1 ./internal/taskview`。

运行使用 `GOMAXPROCS=2`，`GOCACHE=/tmp/noonmark-day-build/cache`、`GOTMPDIR=/tmp/noonmark-day-build/tmp`。日期测试另外逐年覆盖 1…9999 年的一月、三月、十二月 ordinal 往返。所有 Go 结果仅为 focused 投影验证；整体验收、UI、跨平台构建由集成任务另行执行。没有提交 Git commit。

复查修正：Day 保留 recurring 可见轨迹，列表、日期条 pending、统计、趋势统一计入；增加 Day/Calendar 范围差异及普通 kernel guard 不变断言。原始 membership 仍在完整验证后才于私有克隆清除，非法 membership 继续拒绝。

导航联动另经源码核对：Day 前后/今天/日条/键盘只改 selectedDate，Calendar保持独立；Calendar打开Day复制当前日期。只有尚未接通的日期弹窗 gotoDay 同时赋值两者。依据 DayTodoPage.swift:250–272/559/611、NoonmarkStore+PresentationUndo.swift:456–506/524–533、CalendarPage.swift:619–621，不能将 gotoDay 的特殊行为误推广到所有日期操作。
