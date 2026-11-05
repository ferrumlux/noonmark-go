# 只读日历投影方案

## 需求与范围

在 `internal/taskview` 新增纯 Go 月历投影，以显式 TaskGraph、selected、today 为输入，不读写磁盘、不取系统时间、不结算或建立 Day，不依赖 UI、Service 或分类状态。保留原型与生产资料不动。上游依据为 `WorkspaceMonthGrid.swift` 的周一起始、最少五行、月外空格，以及 `NoonmarkEngine.calendarTraces/calendarSummary` 的 DayTodo 过滤与完成条数热度。本批不是完整 Calendar 对等。

## 现状与设计

使用领域 `ValidateTaskGraph` 验证原图，错误原样包裹返回，不伪装空月历。`NewTaskKernel` 为避免修改重复实例而拒绝 recurring，因此只在已验证的隔离深复制中清除 membership，再建立只读 kernel，且以原图 recurring chain ID 集合排除结果；不执行任何命令，不删除链或破坏跨链引用。调用既有 DayTodo，保留 pending、pin、priority 排序及同链延期替代语义。标题取 trace 引用的定义，不取最新定义。

API 为 ProjectCalendar、ShiftCalendarMonth、MoveCalendarSelection。月历提供全部槽位、边界、当前选择详情、可导航状态。日期格提供全部行、前六行及真实隐藏数量、总数和三种主要状态计数、heat=min(completed,4)。空槽没有日期或任务，但仍有格线边界。Position 使用 history/today/future。月份移至目标月一日；日移动使用 UTC civil arithmetic；合法年份为1..9999，最大月份渲染不构造下一年。

## 风险与验证

最大风险是重复实例只读适配误被当成 mutation 授权，因此 adapter 保持私有且只返回 kernel 的 DayTodo 资料，文档明确不证明 Service 支持重复。完整分类历史、重复父实体、洞察、写操作不在范围。UI与持久化路径由集成层另测。

合成单元测试覆盖5/6行、天然4周、全部空槽边界、闰年/世纪、日期及月份极值、非法输入/graph、today与selection分离、热度非完成率、六行预览、pending pin顺序、历史标题、移动与重复过滤、深复制与反复导航无副作用。用外置 build/cache 跑 go test -p=1；不运行 production、Swift 或真实 AI，不把纯投影测试称作 UI 验收。

## 本轮验证结果（2026-11-05）

- `go test -p=1 -count=1 -coverprofile=$NOONMARK_MYGO_BUILD_ROOT/taskview.cover ./internal/taskview` 通过，语句覆盖率98.5%。
- `go vet -p=1 ./internal/taskview` 通过。
- 测试另证明未被替代的 deferred 仍可见、未来 pending 只读可见、选中详情和格内资料一致，以及返回的详情/预览切片互不污染原图。
- 运行环境：隔离合成资料，外置 `/tmp/noonmark-calendar-build`、GOPATH/GOCACHE 外置，`GOTOOLCHAIN=local GOTELEMETRY=off GOMAXPROCS=2 GOGC=20 GOMEMLIMIT=700MiB`。本报告没有声称 UI、持久化或 Swift 对账已通过。
