# GO-RUNTIME-010：旧原生菜单回调越过导航及菜单生命周期

- 状态：独立原反例、候选与正式集成全门禁已通过；修复提交 [6dfc5a1e6a3b76b58b78402655f89e5775f55d39](https://github.com/ferrumlux/noonmark-go/commit/6dfc5a1e6a3b76b58b78402655f89e5775f55d39)。真实 OS／IME 菜单及 Swift oracle 仍待验收，不宣称生产发布。
- 首次发现：2026-11-06（新加坡），普通任务回池菜单候选独立审查。
- 影响：隔离 development 合成任务的菜单动作准入。没有生产资料操作或已知用户数据损坏。
- 引入：应用缺少菜单宿主导航身份来自本批尚未提交的 UI 候选，由本轮实施 agent 编写。原生回调身份缺口存在于锁定 MyGo `977ea57e0baf78da62698ba7371420d192260466` 的 `ui/menu.go`；未定位更早引入 commit 或实际作者，不能用整版依赖归因个人。

## 独立原始反例

1. `TestIndependentOldMenuAfterNavigationRoundTrip`：真实 Tester 打开回池菜单，经正常 `titleTransition` 导航到任务池，再回到相同日与 trace，选择仍保留的旧菜单。旧候选错误执行回池。该反例没有 unsafe 或假造 UI 状态。
2. `TestIndependentCancelledOldNativeMenuAfterReopen`：保存真实 Tester host 的 `chosen` 回调，取消菜单，再打开同 trace 菜单，交付旧回调。旧候选错误回池。反射／unsafe 仅用于保留真实原生回调，未伪造领域事实；不把它冒充真实系统已观察到的回调时序。
3. `TestIndependentOldNativeMenuCannotCrossReattachedSession`：保存旧回调，安全关闭旧 hub/Service，重开相同合成会话并将同 app/UI 树挂到新 hub，再交付旧回调。仅 trace 与从零开始的 navigation 数值不足以阻止动作进入新会话。这是内部重挂防御模型，当前公开 UI 没有此操作入口。

三个生命周期原反例原样保留在 `runtime_return_independent_test.go`。独立审查首先实际判红；只加 navigation 后第一条转绿、第二条仍红，证明不能用成功回调的选择清理条件代替原生动作准入身份。

### 同批能力投影反例

`TestIndependentMenuOmitsCompleteWithOpenSubtask` 另以通过 `core.ValidateTaskGraph` 的合成 open 子任务图证明：仅看 pending 状态会错误展示「标记完成」。源 `NoonmarkEngine.swift:3684–3705` 明确要求 `canCompleteParent`，`NoonmarkStore+PresentationUndo.swift` 仅列出 available 动作。该反例先判红，补上只读能力投影后转绿。当前 UI 尚无新增子任务入口，不把有效图投影测试冒称公开界面新增子任务测试。

菜单与日任务完成圆圈现共用确认图的能力判断，检查匹配 trace、日期／锁日、普通 chain 及 pending／unfinished 子任务；有开放子任务时隐藏完成菜单并禁用圆圈，回池仍可用。图从当前只读投影传入，不重复获取 Service 锁。

## 根因与修正边界

应用原来只在事务完成时比较页面、日期、选中态及 navigation。菜单打开与回调投递属于更早的不同边界：MyGo 按 node ID 与菜单项目位置／label，在下一帧重建闭包。稳定 trace key 会在导航返回或新会话中重新出现。

应用菜单宿主现在绑定 `*titleWindow` 与单调 navigation 身份，标签仍维持原无障碍文案。旧菜单不能跨导航或新窗口实例进入当前动作。新意图即使回到相同页面／选择，也不能让旧事务完成清空它。

锁定 MyGo 的 `menuChosen` 未验证当前 shownMenu，也未一次性消费回调。取消／重开同宿主后的旧回调仍可匹配当前宿主。受审外置依赖补丁现在绑定当前 shownMenu、宿主与仍挂载节点，并一次性消费有效回调；新菜单请求、取消、节点隐藏／禁用、关闭和宿主变化使旧回调失效。原生 popup 同步结束时明确发送取消标记；headless 的 CloseMenu 使用同一通知，不再只丢 host 函数。不能用每帧变动 key 导致合法菜单全部失效。具体补丁身份、定向门禁及平台边界见 `../../../patches/mygo/native-finalize/README.md` 和 `../../tasks/connect-return-menu/DEPENDENCY.md`。

没有修改原参考仓库、官方 module cache 或原 MyGo 工作树；没有新增公开 API／ABI、关闭会话校验、吞掉领域错误或用人工弹层冒充原生菜单。

## 证据与永久门禁

独立红／部分修复日志由审查工位保存：`independent-red-both.log`、`independent-session-red.log`、`independent-partial-fix.log`。独立最终拼合后四原反例与菜单／日视图 race 已通过（`final-independent-race.log`，6.850s）。最终补丁接合后，本工位已重跑完整四模块 race/vet、原生框架门禁（含 ui/root vet）、六目标编译及 Linux CLI，全通过；详情见本批 VERIFY.md。额外完整框架 UI 全套仍只有既有 TestDiffFrameAllocs 红，原 0001／0002 基线同样失败，不把定向门禁冒称全框架绿。

应用还保留真实 Service 持锁暂停时 UI 帧不阻塞、FIFO 发布、标题先保存、双触发、磁盘故障、跨日、另一任务菜单取消／重开等症状门禁。旧原生回调、已确认的领域事务与新界面意图必须分别验证。

## 发行、平台与回滚

系统原生菜单外观、真实显示服务器、各 OS／IME、Swift 动态 oracle 尚未验收。headless 只提供真实菜单项目与回调测试，不输出系统菜单 chrome，不能用应用截图宣称平台菜单圆角已经一致。

回滚仅回滚源码，不能删除或转换回池领域事实；沿用 GO-RUNTIME-009 的开发会话 reader 兼容性及未确认写入保留规则。已完成本地正式集成，不将开发提交写成生产修复。

正式副本2026-11-05 18:52–18:58 UTC复验 exit0：四模块race／vet／gofmt，51项依赖准备，82个ui测试（2.707s）＋root popup（1.046s）及ui/root vet，六目标编译，实际Linux amd64 CLI与day／pool渲染。应用main race52.980s，其他未变模块部分复用Go测试缓存；后续hosted CI使用全新空缓存另验。正式两张回池前后PNG与受审候选逐字节一致，不包括系统menu chrome。
