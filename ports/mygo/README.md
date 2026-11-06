# Noonmark Go + MyGo 移植

这是完整复刻的研发快照，不是功能齐全的发行版。20 场景原型与三批领域模块已集中在此目录；本轮已新增普通任务的真实 UI → 领域命令 → 隔离文件存储 → 重开回读链路。

## 当前可用

- 日待办详情 pending 且日期不早于今天的标题原生多行编辑：700ms自动保存，Return换行，导航／完成／退出先确认草稿，失败保留输入；历史／完成标题保持只读。
- 共享后台写入lane与UI已确认读视图，存储提交不阻塞重绘；新原生输入不会被迟到完成或跨窗更新覆盖。
- 日待办列表／详情和任务池详情的真实子任务：新增、700ms 标题保存、难度菜单、删除；当天实际子任务可完成／撤销完成，任务池 planned 子项不显示完成操作。父任务仍须显式完成。
- 子项独立输入所有权、原对象版本守卫与失败草稿恢复；旧菜单、迟到确认、跨窗口删除／回池和关闭失败不会把草稿写到新对象。详见 docs/tasks/connect-subtask-ui。
- Day 列表和详情显示真实 lineage 难度加权进度与历史下限，跨窗共享已确认投影，失败不发布未保存百分比；0／100及历史状态沿原版展示规则。手动进度滑杆尚未接通，现有手动值只读。

- 在 Day Todo 所选今天/未来日期输入中文标题，Enter 提交；任务池可用Enter或添加按钮。未来任务不会误写到今天。
- Day 的“从任务池排期…”弹窗可将已有任务排到所选今天/未来日期；任务池页仍有今天/明天快捷排期。
- 完成/撤销今日任务，按已确认事实更新导航；普通任务徽标沿原版按今日pending、池化链及历史链投影，完成子项可计入已完成链，零值不显示。Completed／Unfinished页面仍是有限trace列表，完整层级展示尚未接通。
- 日任务行原生右键菜单接通「回到任务池（留下轨迹）」：先保存标题，今天保留轨迹／未来取消，失败保留草稿和选择；没有回池 undo。
- 普通 Day 行菜单接通「延期到…」与历史未完成任务的「延续到…」：日期先保持草稿，经输入屏障和共享后台队列确认存储后才导航；在途目标不可更改。保存期间明确显示「正在保存」与「关闭」，关闭只撤展示，不撤销已提交的操作。跨窗口晚到输入保留在原任务的可达恢复入口，不自动新增或搬到新轨迹。此批仍无 snapshot undo／成功 toast，详见 docs/tasks/connect-carryover-ui/VERIFY.md。
- 原版14日锚点日期条、默认平铺/时间排序、动态长标题行、所选日期的真实统计/七日趋势与可折叠详情；日期条固定，输入和长列表独立滚动。
- 日切换、未来到达后的完成与历史锁定、快速记录到任务池。
- 普通任务真实只读月历：5/6行、日期/月份键盘导航、完成数量热度、可展开日期详情及打开 Day Todo。
- 关闭并重开开发会话保留完整 TaskChain / Definition / Trace；自然日推进时以同事务结算任务和空分类历史。
- 写入失败保留草稿和原快照；并发冲突、损坏和不完整写入 fail-closed。

未接入的页面明确显示状态，不混入静态演示任务。原 20 场景视觉原型仍可显式运行，详见 docs/UI-PROTOTYPE.md。

## 开发运行

构建要求 Go 1.27.3、Bash 和 Python 3.10+（运行程序无需这些工具）。MyGo 与依赖已锁定；受支持 scripts 会从官方 ZIP 校验并准备外置 native-input patch，不修改 module cache。裸 Go 命令不包含所需 API，不是支持的构建入口。所有缓存、二进制、运行数据和截图必须位于仓库外，build root 必须已存在且为物理绝对路径。

```sh
mkdir -p /absolute/external/noonmark-build
export NOONMARK_MYGO_BUILD_ROOT=/absolute/external/noonmark-build
export NOONMARK_GO=/absolute/path/to/go
bash scripts/check
bash scripts/build
# v2-session-001 必须不存在；此入口只创建隔离 development 会话
"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -init-session "$NOONMARK_MYGO_BUILD_ROOT/v2-session-001"
# 关闭后重开同一会话
"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -session "$NOONMARK_MYGO_BUILD_ROOT/v2-session-001"
# 不接存储的旧视觉原型
"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -prototype
```

`scripts/check` 同时执行合成依赖准备门禁及已修复 native-input 定向回归。完整框架 `scripts/check-mygo-native --all-ui` 仍有与未修改基线相同的 TestDiffFrameAllocs 阈值失败，详见 docs/tasks/connect-title-autosave/DEPENDENCY.md；不能称整个框架全绿。

`scripts/check` 还运行固定 Unicode16 官方完整规范、离线重生成、输入／输出目录保护负例及六目标纯 Go probe 编译。ICU76 差分须显式调用 canonical16 独立入口，不把普通 Ubuntu CI 当 ICU76 或 Swift 执行证据。

默认 `scripts/check` 已包含365天／762任务的年度中段合成回归：今日34行真实滚动、延期／延续、历史不变及重开对账。fixture 每次新建，测试进程可取消并确认回收；这不是原版完整一年故事或真实桌面验收。可用 `scripts/test-carryover-year` 另存隔离证据。

Linux 原生窗口需要 GTK3 和可用 DISPLAY / WAYLAND_DISPLAY。无显示服务器可通过 -render-one 在相同 MyGo 代码上生成离屏帧，但不算真实桌面验证。scripts/cross-build 编译 macOS / Windows / Linux 的 amd64 / arm64 六目标，不代表这些平台已运行。

## 模块与边界

- domain-core：TaskChain / Definition / Trace、精确时间与历史不变量。普通任务链放弃／原位重新激活已具备纯核心API与独立回归，但尚未接Service、reader或UI；新历史事实仍拒绝落盘。
- domain-extensions：分类、重复计划生命周期及版本历史，尚未接入运行 UI。
- domain-extensions/classification/canonical16：固定 Unicode16 名称算法与可重现数据表，仍是独立候选；未替换分类 engine adapter，不能外推为 Swift 互通或开放非空 catalog。
- domain-codec：候选 v7 规范编解码及安全校验，只用合成 fixture。Swift 互通尚未验证，非空分类 audit 默认拒绝。
- internal/inputdraft：按原版移植的纯 IME autosave gate，已用于日待办标题编辑。
- internal/titlecoordinator：实际多窗口标题／导航／关闭协调与有序lane；每次真实退出仍重新检查原生输入。
- internal/titlesession：草稿/实际存储确认/生命周期快照状态；拒写保留输入，不能单凭状态允许退出。
- patches/mygo/native-finalize：可重现的帧间 native snapshot/finalization 修复，已接日待办标题；三平台实机 IME 仍待验收。
- internal/taskapp 新增独立的普通任务多事件历史 inspector、确定性追加和旧历史前缀检查候选，尚未接入 reader、存储或 UI；不能因此在现有会话写入放弃／重新激活历史。详见 docs/tasks/connect-history-chain-service-audit。
- 同一历史候选新增 typed 日结束、完成／撤销完成精确审计，包含前置图／字符串预算、完整命令重放及旧事件前缀保护；仍未接入现有 Service、reader、存储或 UI。详见 docs/tasks/audit-history-commands。
- 同一未接线候选补充废弃／重新激活的 typed 精确审计：区分请求与实际目标，并按来源派生 A 或 U 事件，保留旧历史；仍不开放现有会话中的新生命周期动作。详见 docs/tasks/audit-history-lifecycle。
- 同一候选补充标题与手动进度的 typed 精确历史审计，保护原版版本／复用、未消费参数、历史进度下限与零事件编辑；仍未接 Service、存储或 UI。详见 docs/tasks/audit-history-edits。
- internal/taskapp：仅普通任务的开发会话事务与持久化，含专用审计的标题编辑、回池和普通子任务 API；日待办标题、回池菜单与子任务 UI 已接入，今天回池／未来取消及子任务命令均经过持久化故障矩阵，回池 undo 尚未实现。详细安全和容量边界见其 README。
- docs/migration-audit：完整产品差距与上游测试库存。

还未完成 SQLite schema 19、同步、AI、全局快捷键、手动进度滑杆及完整编辑/分类/重复/飞光运行接线、原生时区/唤醒通知完整对等、真实一年互动演示及原生三平台验收。当前文件后端不能替代生产存储；不得打开、探测或迁移实际用户数据。

## 来源与许可

AGPL-3.0。上游 https://github.com/quboliu/noonmark，基线 43e6eefaae3a04be89cab3e234d470111afd80ef。MyGo 保留 MIT 许可。该独立仓库保留 ports/mygo 布局以便日后通过获准的上游 fork 分支提交 draft PR；目前未创建上游 PR。

开发会话格式已更新为 v2，以保留未分类任务也必须具有的分类历史事件。旧 v1 会话会明确拒绝打开，原资料不变；请另建全新 v2 开发会话，不执行生产迁移。

已增加 GitHub 开发 CI，见 [GITHUB-CI](docs/GITHUB-CI.md)。回池／取消扩大同一 v2 reader 的严格接受范围，旧 reader 会拒绝含新事实的会话；回滚源码不能删除或转换会话来迎合旧版。详见 [TASK_RETURN_BATCH](docs/TASK_RETURN_BATCH.md)。

普通任务延期／延续已有隔离 Service 持久化命令和 Day 行菜单接线。支持范围、deferred reader 扩展和回滚不兼容说明见 [connect-trace-carryover-service](docs/tasks/connect-trace-carryover-service/README.md)。
