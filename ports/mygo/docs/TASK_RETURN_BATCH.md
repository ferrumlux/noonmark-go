# 普通任务回池：领域／事务批次

## 需求、现状与边界

在 `7e816a7986dffbf10ea5f5a9dbefb63beac2e8d2` 的隔离副本补上普通任务回池。上游来源为 `quboliu/noonmark` 的 `43e6eef`。保留 AGPL-3.0-only，不改原始仓库、正式副本或 MyGo；只用合成资料。当前 runtime 原先只有创建、安排、完成、日结与标题命令；不能将已存在的状态枚举当作已接通的能力。

## 源码契约

- `Sources/NoonmarkCore/NoonmarkEngine.swift:2019–2065`：只接收未锁日的普通 pending trace。过去日期不可改，今天变成 returnedToPool，未来变成 cancelledDraft。两者清 pin、写 settledAt/contentUpdatedAt 并 touch chain。
- 今天保留内部轨迹事实，立即捕获一次空分类快照；不再显示在 Day Todo。未来写新的 cancellation ID 与取消日期，不捕获分类，隐藏取消轨迹。未来取消不是今天留下历史轨迹的同义词。
- 同文件 `materializePoolDraft`：总是建立下一 sequence 的定义，以当前定义标题、trace 说明及 pending/unfinished 子任务生成计划子任务；保留 lineage，重新编号位置并分配新 ID。旧定义仅更新 supersession 元数据。trace 备注复制到 chain。
- `prepareSubtasksForHiddenTrace`：未来隐藏轨迹中已 cancelledDraft 的子任务恢复 pending，并清完成／结算／取消标记；这些取消子任务不进入新的池计划。
- `settleDays`：日结只为 pending→unfinished 和 completed 捕获快照，不能重复捕获 returnedToPool。已有回池事件时间仍为回池时刻，不改成后来的日锁时间。
- `App/NoonmarkMacApp/NoonmarkStore+PresentationUndo.swift:359–420`：今天 pending 的菜单还包含完成、置顶、推迟、改为新任务等；未来 pending 包含置顶、重新安排、回池。不可假装这些尚未实现的菜单动作可用。
- `DayTodoPage.swift:1272` 和 `AppCopy.swift:103`：回池菜单中文为「回到任务池（留下轨迹）」；上游当前和未来共用此文案。`NoonmarkStore+TaskMutations.swift:917` 对未来使用 snapshot undo，今天使 undo 失效。此批未实现 undo。

## 方案与事务证明

`core.ReturnToPoolInput` 显式携带 trace、chain、新定义、取消及子任务 ID；命令可确定性重放。额外 chain 绑定保护陈旧或错链 UI 回调。kernel 深复制、完整图验证后才发布。`NewTaskKernel` 原本已拒绝任何 CycleMembership；此批另补命令级显式 recurring 拒绝，并放在普通日期／状态检查之前，防止未来扩大构造范围时漂移上游语义。测试覆盖当前、未来、过去与锁日的内部合法图，不将其描述为现有公开入口可达漏洞。

`Service.ReturnToPool` 在单一服务锁内取日期、产生单调时钟和身份，执行命令、捕获分类、完整编码／解码校验及 append-only commit，然后发布图。磁盘失败不发布候选，也不补写或绕过 O_EXCL。

`captureReturnHistory` 先验证旧完整快照与 before 一致，再用相同输入独立重放命令，对比整个 after 图。只有精确匹配才追加今天的单一空分类事件。没有共用任意字段豁免表；原通用 `CaptureEmptyHistory` 仍拒绝引入回池／取消状态，冻结全部既有定义，禁止为已有 chain 泛用新增定义，并额外冻结未锁日上的既有 terminal 回池事实及其实际子任务。独立审查发现的四个边界反例、根因与门禁见 `engineering/failure-cases/GO-RUNTIME-009.md`。

`ValidateEmptyHistory` 只增加两种严格子集：returnedToPool 必须有一个 settledAt 时刻的匹配事件；future cancelledDraft 必须严格晚于取消日期，且没有事件。其余 catalog、recurrence、ideas 与未接通状态继续 fail-closed。旧锁日的 completed／unfinished 检查保持不变。

`Service.Schedule` 为当前定义中各计划子任务分配新的实际子任务身份，避免回池后存在计划子任务时无法再安排。

## 验证与交付范围

新增 core 和真实 Service 合成测试覆盖今天／未来、重复、过去／锁日、完成、错链、旧时钟、缺 ID、开放／完成子任务筛选、隐藏取消子任务、回池→再安排、跨日→再回池→改标题→重开、完整快照篡改、缺审计、terminal 泛用修改及四阶段磁盘故障。

此批没有新增 UI 控件、菜单或 CLI 动作，不声称原生用户操作已接通。现有 UI 的池／日投影可读取结果，但用户不能从新菜单发起此动作。没有执行 Swift oracle、真实 OS/IME、iCloud、Keychain、AI 或生产资料操作。

## 会话兼容性与回滚

开发会话 wire 仍为 v2，但此批 reader 的严格接受范围新增 returnedToPool／cancelledDraft。旧版 runtime 会 fail-closed 拒绝含这些新事实的开发会话；回滚源码不代表旧版本能继续打开新会话。不得删除、覆盖或转换这些事实来迎合旧 reader。保留原会话与错误证据，使用理解相同事实范围的版本，或另行建立全新的隔离合成会话。本批只交付 Service 能力，没有菜单入口或 undo。

## 下一批原生 UI

先按上游 TaskContextActions 的结构接真实菜单，保留原版间距及圆角，不添加临时 toolbar 按钮。选定 trace 与 chain 使用不可变动作键进入共享 titleHub commandOnce；通过共享 flush/navigation 队列先确认标题持久化，再写回池事务。事务完成只发布经过确认的只读图，并只清除仍匹配的旧选中项。必须覆盖两窗口重复触发、保存中回池、失败／重试、取消／重开、迟到导航和跨日。菜单仅呈现真实接通动作；未来 undo 需要独立能力批次与审计，而非假按钮。

## 正式集成证据

实现提交：[c984b7a013a0b8ba51faa5cbfc425868f4bcc637](https://github.com/ferrumlux/noonmark-go/commit/c984b7a013a0b8ba51faa5cbfc425868f4bcc637)。2026-11-05 18:28–18:30 UTC，正式副本完整 scripts/check、六目标 cross-build 和真实 Linux amd64 合成 CLI 门禁 exit 0。独立审查的原始反例完整保留；本批尚未包含菜单或 undo，不以该结果代替后续 UI／OS 验收。
