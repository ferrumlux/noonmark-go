# 普通任务链废弃与原位重新启用：纯核心方案

task-id: connect-history-chain-core

## 需求与现状

本批为 A 级隔离领域移植。正式只读基线是 `7c239df6941f8e6f923b078da6e6ba5ce8dc10b3`，上游只读来源是 `quboliu/noonmark` 的 `43e6eefaae3a04be89cab3e234d470111afd80ef`。依照上游 AGENTS.md 的需求、现状、设计、实施、验证顺序，在 sibling 独立源码副本实施；本轮明确指定的新副本与外置 BUILD 取代上游针对另一台 Linux 主机的历史路径规定。资料全部合成，无 production、iCloud、Keychain、AI 或 GitHub 写入。

现有 TaskKernel 已有普通任务、子任务、延期及延续命令；每次通过 mutex 内深复制候选、ValidateTaskGraph、验证后发布。构造器拒绝任何 CycleMembership。已有 HistoryProjection 能描述 abandon/reactivate intent，但不是相应 Service 或 UI 接线的证明。原状态枚举和通用 v7 codec 的表达能力也不是 runtime reader 已接受事实的证明。

## 先于实现确定的接口

- `AbandonChain(AbandonChainInput{ChainID, TraceID, Now}) error`：ChainID 是额外的 Go 路由围栏，必须匹配请求 trace，不能通过历史请求或 activeTrace 重定向跨链。没有 Today，因为来源没有自然日或锁日拒绝条件。
- Abandon 的 nil error 只表示实际目标和 chain 已完整验证并改变，不表示请求的历史 trace 已改变，也不返回该历史 ID 来暗示目标。错链在重定向、链状态及内容时钟检查之前拒绝；未知请求 trace 返回 ErrNotFound。
- `ReactivateAbandonedChain(ReactivateAbandonedChainInput{ChainID, TraceID, Today, Now}) (DayTraceID, error)`：成功返回指定的同一 trace ID；失败一律返回空 ID。Today 必须为可表示的合法 LocalDate。
- 两者只新增纯核心文件；不修改 validator、现有 Service、codec、UI、empty-catalog scope 或聚合入口。不产生新身份、trace、definition、day、subtask 或 note。

## 源码合同

1. `Sources/NoonmarkCore/NoonmarkEngine.swift:2513–2534`：先取得请求 trace 与 chain，非 active chain 返回 chainAbandoned；目标为该链 activeTrace，若没有则使用请求 trace。目标必须 pending 或 unfinished。仅把目标变 abandoned、清 pin、写 settledAt=Now 与 contentUpdatedAt=Now，chain 变 abandoned、updatedAt=Now。请求历史 trace 若不是实际目标，完整保持不变。没有“最新历史前沿”、未锁日、日期或池位置的额外资格检查。
2. 同文件 `3814–3819`：activeTrace 只指 pending，并按 createdAt 降序选择，不是 unfinished、最新内容时钟或所有历史事实。Go 有效图最多一个 pending，故顺序无歧义。
3. 同文件 `2537–2572`：source 与 chain 必须同时 abandoned，并拒绝同链任一 pending。重新启用原 ID；locked 日优先变 unfinished，即使自然日倒退；否则 source.date>=Today 变 pending 并清 settledAt，过去变 unfinished。unfinished 保留原 settledAt，仅 nil 时以 Now 填充。所有路径清 pin，chain 变 active。
4. `Models.swift:468–469、1037–1053`：trace 与 chain 都是 ContentMutationClock.advance，要求旧时钟、Now 有限且 Now>=旧时钟，允许相等并保存传入的精确 Date bits。不能用 touchChain 的 max 语义掩盖倒退。其他未变实体的较新时钟不构成本命令的时钟拒绝依据。
5. `NoonmarkEngineTests.swift:1801–1830、1832–1898、3868–3891`：废弃后留在未完成池、开放子项保留、原位重新启用、历史请求重定向 active trace、锁日与自然日倒退。

## 不变量与拒绝边界

继续经 mutate 全图校验，失败原子回滚；不得为通过新增测试弱化 ValidateTaskGraph。ordinary scope 保持与构造器一致，命令还对所选 chain 的 CycleMembership 显式返回 ErrUnsupportedRecurring，防止未来构造器扩展悄悄改变本批范围。

有效图中 abandoned trace 必须已有 settledAt、没有 pin，因此 nil settledAt 补齐及非 nil abandoned pin 清理是来源防御语义，不可用无效输入绕过构造器来声称覆盖公开路径。实现仍保留来源 nil 分支。

来源 abandon 没有历史前沿 guard。若选中已被后续轨迹引用的 unfinished 源且没有 pending，改为 abandoned 会破坏现有 continuation 拓扑；本纯核必须返回既有 ValidationError 并保持原图，不擅改其他 trace、重新连接或放宽不变量。上游 `TrajectoryTopologyValidator.swift:228–246` 同样不允许 abandoned 作为 continuation source。该拒绝是 Go 有界图发布保护，不声称 Swift abandon 函数本身拥有同一拒绝或错误顺序。相同原则适用于任何候选图验证失败。

## 池中废弃是另一条 API

`Models.swift:1268–1305` 的 actionPlan 将 active trace、历史 unfinished、abandoned 和任务池动作分开。`App/NoonmarkMacApp/UnfinishedPoolPage.swift:146–149` 的 abandonPooledChain 调 `store.deletePoolTask`，后者在 `NoonmarkStore+TaskMutations.swift:1435–1450` 调 `candidate.removeTaskFromPool`。`NoonmarkEngine.swift:1486–1509` 只废弃 chain，保留 trace 并处理移除当前分类；无 trace 时转到 deleteUnscheduledTask。它不是 abandonChain 的别名。本批不实现、不模拟、不把历史请求自动转换成该 API。

## 后续 Service 必须独立完成的审计

纯核没有 classification state，不捕获或伪造事件。正式 reader 的空 catalog 边界保持原样。未来 Service 需在同一候选事务：

- 绑定请求 ChainID/TraceID 与修改前事实；独立精确重放并比较整个 after 图，确认 abandon 的实际目标可能不同于请求历史 trace。禁止通用历史字段豁免或仅按请求 ID 捕获。
- abandon 必须为实际 abandoned 目标调用 capture；`TaskClassification.swift:4022–4059` 使用当前分类、目标新状态、Now 与 revision+1，追加 snapshotEventsByTraceID，替换该 trace 的 latest，并保留已有事件。其他 trace 的事件、latest 和所有其他 supplement 都不变。
- reactivate→unfinished 同样追加一次新 unfinished capture，并替换 latest；capture 的 Now 可能晚于保留的 settledAt，不能错误要求两者相等。反复 abandon/reactivate 需要保留完整序列，不可覆盖旧事件。
- reactivate→pending 不调用 capture，不删除旧 abandoned latest 或事件，也不减 revision。历史分类保留与当前 pending 状态暂不相等是上游行为，需专用、精确的保留规则，不能泛用允许任意 mismatch。
- `TaskClassificationModuleTests.swift:1880–1923` 明确验证 abandon→same-day reactivate→再次 abandon 保留两次 abandoned 事件；纯核反复切换测试不等于该分类合同已实现。当前 `internal/taskapp/history_test.go` 对 unsupported abandoned event 的拒绝门禁必须保持，不能为了回读新增状态扩大 reader。
- 重新核验当前 catalog 为空、所有完整 supplement、既有日锁与历史事实、跨日结算次序、时钟 frontier、v7 编解码与磁盘 prefix，覆盖失败原子性后才发布。自动分类资格变化、snapshot undo、UI 标题保存/路由与实际菜单由独立批次负责。

## 验证、风险与回滚

测试先以现有领域命令产生正常历史，再为命令无法构造的边界建立明确标注、先通过 ValidateTaskGraph 的合成结构图。覆盖 pending/unfinished、历史重定向、所有状态拒绝、错链与未知身份、重复操作、active 冲突、锁日/过去/今天/未来、精确相等/负值/极端时钟、两实体时钟倒退、pin、notes/children/其他事实不变、完整输出 clone 隔离、并发与验证后失败回滚。

固定 Go 1.27.3；缓存与日志在外置 BUILD，GOPROXY/GOSUMDB=off，低并行与内存上限。不做重型 release 或清理缓存，只跑 domain focused/race/vet 与相关 codec/projection 兼容回归，完整聚合由整合阶段整合后另验。来源驱动的 Go 测试不等于 Swift oracle；不声称跨平台实机、完整 App 功能或持久化已接通。

本批没有生产/磁盘发布或流量，因此灰度和运行监控不适用；候选的风险控制为独立源码副本、严格 scope、未接线与精确测试证据。回滚为不整合这些新增文件。未来一旦 Service 持久化新事实，不能删除或改写会话来迎合旧 reader，必须另评估版本兼容。
