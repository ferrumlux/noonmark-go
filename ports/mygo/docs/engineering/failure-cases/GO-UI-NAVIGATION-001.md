# GO-UI-NAVIGATION-001：导航计数错误复用页面行数及显示零值

状态：已修复（本地隔离验证；未推送或发行）。修复 commit：cf6e6fbfc99cb669a8da012d2d3f2938658f36f9。首次发现：2026-11-05。

- 症状及影响：空库侧栏显示五个 0；今日任务完成后今日徽标仍为 1；父任务仍 pending 但已有完成子任务时完成池徽标为 0。未完成池也把 trace 行数误作 chain 数，遗漏 abandoned 候选、未排除已有 completed trace 的 chain。
- 引入 commit：4cbd56883b8a6065c82902923b66ecc0741a2cef。Git author／committer 均为 ferrumlux；实际操作者无法由现有证据确认，未知。不得将 Git identity 当成真实操作者。
- 破坏机制：runtime_ui.go 使用 len(rowsFor(...)) 作为五个导航徽标，并无条件创建数字文本。rowsFor 是当前页面的有限行投影，不是 Swift navigationCounts 的领域计数契约。
- 原始取证：真实隔离 Service 命令加 MyGo 原生 renderer 的三个 TestNavigationUI 门禁在旧代码产生三红；外置 evidence/navigation-red.log 与 red-* PNG 记录症状。相同测试在首轮修正产生 navigation-first-green.log。截图是 headless MyGo 产物，不是 Swift 或 OS 实机。
- 已排除：不是持久化丢失、完成命令未执行或子任务未完成；测试在点击后核验真实 Service facts。不是 progress 百分比问题，原始加权投影的 25% 仍然正确。
- 根因修正：建立普通任务导航投影，与已审核 progress cache 在成功发布图边界共同建立；不在每帧 NewTaskKernel 或 clone 全图。day 使用 today pending；pool 按 current definition 和最新 trace；unfinished 按普通 chain 去重且排除任何 completed trace；completed 包含可呈现历史中的完成子任务并按普通 chain 去重；未来为当前 Service 普通 pending 的未来日期投影。零值与 compact 不创建徽标。
- 来源：Swift 43e6eef，NoonmarkStore.navigationCounts、WorkspaceShellViews.NavItem，以及 NoonmarkEngine.getDayTodo/taskPoolCount/visibleFuturePlans/unfinishedPoolCount/completedTaskHierarchyCount/ChainTraceFacts。
- fast gate：TestNavigationProjectionSourcePredicates、TestNavigationConstructorOnlyAtPublication。
- symptom gate：TestNavigationUIEmptyHidesZero、TestNavigationUITodayOnlyPending、TestNavigationUICompletedChildCountsChain；其余 TestNavigation* 覆盖回池重排、未来取消、今日切换、失败重开、双窗 FIFO、无变化/重复帧、无障碍及长列表。
- 聚合入口：scripts/check 强制运行，gates.tsv 追加注册；六目标编译及 Linux CLI 是补充，不代替症状门禁。
- 范围不足：completed/unfinished 页面仍是现有 trace 子集，不因此成为完整 hierarchy 页。recurrence 窗口、飞光、分类等不支持计数不造数；不改 core、Service、codec、原 MyGo 或资料格式。UI 文案与输入机制不改。
- 发行／回滚：尚未发行。只撤回本任务 owned UI/cache/test/doc 文件及注册增量；不触碰资料。正式整合 commit 确定、完整门禁和独立审核通过后才将案例关闭。
- 可执行教训：页面展示行数不能代替导航领域投影；同时验证零值缺席、完成子项、历史链去重和发布边界，不能只看截图有没有数字。

2026-11-05 候选验证：相同冻结 Go 源码完整 scripts/check exit 0（主包 race 186.086s）、六目标 compile exit 0、Linux CLI 及隔离 session 创建/2x render/重开/2x render exit 0。所有原始症状门禁及扩展回归通过，已查看空库/完成/完成子项/长列表/compact 原生截图。仍待独立复审与正式 exact commit，状态不变。

正式闭环：独立复审通过；正式副本scripts/check退出0（root race167.197秒）、六目标编译及Linux隔离create/reopen/day+pool渲染／拒绝覆盖门禁退出0。原三个症状由红转绿，新增独立测试已纳入永久门禁。没有删除旧断言或放宽资料验证。以上候选“待复审”措辞为当时记录；此节为最新状态，范围仍不含完整hierarchy列表与Swift／OS实机。
