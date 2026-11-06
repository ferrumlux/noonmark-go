归档说明：以下为本批设计或隔离验证的阶段记录，其中待实施／待整合措辞指当时状态。原文 SHA-256：`d8d0391913f3aea0f7bb5dec391654624b0fe72785abf23797b514d9b21a3f2d`。归档仅调整协作措辞与任务标识；正式整合结果另见本目录 README。

# 日结束与完成命令：未接线精确历史审计设计

task-id: audit-history-commands

日期：2026-11-06。级别：P。状态：R2 设计候选，修订独立审查唯一阻断 R1，待只读复核；设计作者没有实施、构建或测试运行。§7另列整合阶段已完成的 Go 单侧观测，不混作本阶段验证。

R1 原文及 SHA `aa9ad33f933e37bad9c39a8b9a6f33012a5ad8db431458465dcf1628f6f73dbf` 原位保留。本修订仅补 caller graph / EventIDs 的前置预算、验收阶段及相应来源测试，不改变已定 typed replay、capture、clock 或未接线边界。

## 1. 本次只交付什么

在已 accepted 的三个 history vNext 纯 helper 之上，新增**独立、仍未接线**的 typed 日结束（`SettleTaskDays`，下称 settlement）及 complete/undo 精确 before/after 审计。它们为后续 Abandon/Reactivate writer 提供前置证明，不实现该 writer。

- 只允许新增 package 内部命令输入、精确 replay/组合 helper、封闭结果类型和测试；本次设计不改 accepted r1 三 helper。
- 不改 `CaptureEmptyHistory` 的 generic locked/terminal 冻结，不加 `allowLockedChanges`、`captureTraceID`、任意 status 或字段豁免。
- 不接 `Service`、reader、store、codec callback、UI；不改 v2 marker/record 或 v7 wire；不实现 clock collector、v3 writer、完整 command log、导入或历史修复。
- 不运行/读取 production、真实会话、home、iCloud、Keychain、真实 AI；远端写保持暂停，不重试或绕行。
- 本设计及本轮文件仅位于 `/workspace/shared/noonmark-history-command-audit-design-20261106/`。正式树只读，未复制工具链/cache。

## 2. 冻结基础与证据

正式路径：`/workspace/scratch/586e77bd59a6/noonmark-mygo-work/noonmark-go`；R2 重新读取 HEAD 为 `03a18cca51766e1d6b501e5e015ce96d0be5a7c7`，`git status --short` 无输出。六个 history helper/测试文件及相关文档已提交。R1 当时的 `b8857778...` 与未提交/聚合状态仅是历史基线，不能继续当成当前状态；本轮仍只读正式树。

上游只读 sibling `noonmark-reference`：`43e6eefaae3a04be89cab3e234d470111afd80ef`。遵守其 AGENTS、`docs/agents/domain.md`、相关 CONTEXT 领域词汇与 ADR-0022/0037，以及 Go `ports/mygo/AGENTS.md`。未发现这两棵树的 repo-local SKILL.md；已读软件工程 skill。

已复核 SHA-256：

- 本次独立设计审查：`/workspace/shared/noonmark-history-command-audit-review-20261106/REVIEW.md`，SHA `a3b9f83aa982ba7098d877ef82764db439a7277ae9a4ee38f842c16d6b90a896`，仅 R1 输入预算须修订。
- accepted R3 设计：`e45ec63d1766dd6c74f1826a500333778cc8022797b7aa271a8b2618377839a3`。
- 独立 REVIEW：`e8275f146451ac2087082f1e3681e3c960ef0d0546c724af8f925ff2f260d2b5`。
- `history_vnext.go`：`d59969205f42ac832a8824fb0cb2266869c43eeaf28f867d8565879bd16ecd86`。
- `history_vnext_builder.go`：`bc3713653051869c918f666407da75b96a3c3096d7b406975c29ac59a8caa64d`。
- `history_vnext_machine.go`：`44ce117f37f7705c5ece5380b8cfe88c429b18b3868266cd9f7a8e21f5be8715`。

本轮结论为**源码读取及结构推理**。R1审查指出的是拟新增入口的前置资源承诺缺口；R2没有运行内存耗尽、domain red/green或性能实验，也不声称修复了现有受控Service。既有 r1 的运行证据属于上述 REVIEW，不是本设计重跑。本轮没有发现已运行复现的 r1 缺陷，也没有以结构推理修改源码。

### 精确源定位

下列 Go 路径从 `ports/mygo/` 起；Swift 路径从上游 `Sources/NoonmarkCore/` 起。

| 来源 | 使用的合同 |
| --- | --- |
| Swift `NoonmarkEngine.swift:3017–3069` | dates/trace capture Set、新锁 P→U 与已有 C、既有锁跳过、锁时间使用同一 now |
| Swift `Models.swift:18–22`；Go `domain-core/core/models.go:54–79` | X 不 formsDayHistory；不能凭 X-only 日期生成锁 |
| Swift `TaskClassification.swift:1436–1451,4022–4056` | 默认 UUID、空分类仍 capture、调用者 now、stream append/latest 替换、revision+1 |
| Swift `NoonmarkEngine.swift:1989–2017,3684–3749,3805–3811` | complete/undo 资格、无 capture、真实 completedAt；touchChain 用 max |
| Go `domain-core/core/kernel.go:34–58,235–370` | clone/mutate 验证后发布、完整 completion 与 settlement 任务事实 |
| Go `domain-core/core/time.go:14–36,65–98,107–150` | bit exact、数值时序、合法相等 now、NextMutationInstant 和日期端点 |
| Go `domain-core/core/validation.go:232–351` | 状态/clock/引用/拓扑，不用结果 helper 取代 graph 校验 |
| Go `internal/taskapp/history.go:139–169,213–339` | 旧 prior/before 绑定、generic 冻结、after-only U/C、date/ID 排序和随机 ID |
| Go `internal/taskapp/{return.go:62–125,carryover.go:77–139}` | 独立 whole-aggregate replay；旧单 capture map-pair 追加不可照搬为多事件支持 |
| Go `internal/taskapp/{manual_progress.go:102–178,subtask.go:184–262}` | 精确 routed input、整图比较、supplement 保留及额外固定围栏 |
| Go `domain-codec/canonical/canonical.go:41–45,254–296,322–412`；`domain-codec/v7/schema.go:33–44`；`domain-core/core/models.go:110–194` | 固定 bytes/nodes/depth、keys 算节点、字符串编码不缩短有效 UTF-8、typed 结构与字段预算 |
| Go `domain-core/core/validation.go:33–65,68–88,94–101,193–204`；`domain-codec/v7/domain.go:22–59` | FromDomain 先 graph Validate/索引再建 AST，因此必须在外层先 gate caller graph |
| Go `domain-codec/v7/domain.go:16–91` | FromDomain 完整九段、Supplement 克隆、ToDomain/Validate；FromDomain 不是完整历史认证 |
| Go `internal/taskapp/history_vnext.go:36–59,69–189` | 私有只读 prior proof、bounded 完整 inspector；必要一致性，不是写权限 |
| Go `internal/taskapp/history_vnext_builder.go:11–117,130–169` | 显式 intents、deterministic append、classification-only candidate、旧流/锁前缀 |
| Go `internal/taskapp/service.go:49–132,149–236,299–306` | reconcile/probe/发布 Today、当前 generic completion、时钟 seam |
| Go `internal/taskapp/store.go:237–333,387–451` | reservation 前后边界、blocked、不确定提交、旧 v2 gate/prefix |

## 3. 三层合同不可混用

1. **inspector**：完整 snapshot 在固定 forward-clock ordinary profile 内满足必要条件。不证明 aggregate 历史可达、动作来源或授权。
2. **本阶段 typed audit**：给定已验证 prior、精确 before/after、明确命令及时间/IDs，after 必须恰等于一次当前 Go purecore 命令重放，并只有规定 captures。它证明这个边的精确性，不证明 prior 从空库可达，也不是持久化许可。
3. **未来 Service/store**：绑定会话、写锁内真实 Today、全部持久 facts 的严格前进 Now、精确 prior、一次提交及确认后发布。用户授权、并发、磁盘结果不由纯 proof 替代。

例如 r1 可接受的 `Ux@10, Az@10, Uy@10` 不代表一次真实 settlement 被正确分批。本阶段既不改 r1 去补整图可达性，也不让上述必要条件替代一次输入绑定的完整 replay。

## 4. 最小输入与封闭结果

建议新增 `history_vnext_commands.go`，只含两个审计入口；命名可微调，语义固定：

```go
type settlementHistoryInput struct {
    Today core.LocalDate
    Now core.Instant
    EventIDs []string
}
type completionHistoryAction uint8 // 仅 complete / undo，零值及未知值拒绝

type completionHistoryInput struct {
    Action completionHistoryAction
    ChainID core.TaskChainID
    TraceID core.DayTraceID
    Today core.LocalDate
    Now core.Instant
}

auditSettlementHistoryVNext(prior validatedHistorySnapshot,
    before, after core.TaskGraph, in settlementHistoryInput) (auditedHistoryCommand, error)
auditCompletionHistoryVNext(prior validatedHistorySnapshot,
    before, after core.TaskGraph, in completionHistoryInput) (auditedHistoryCommand, error)
```

- completion 的 `ChainID` 是本 adapter 的显式 owner 绑定，不改变 core API：先确认 `before[TraceID].ChainID == ChainID`，再仅对该 TraceID 调用 complete/undo。不得搜索“同链另一个合适目标”、根据 after 选择动作或 silently retarget。未来若保留 Service 的 trace-only UI API，应在同一锁内从已绑定 before 派生 owner，不能让此输入暗示 UI 已接通。
- settlement 的调用者只提供恰好所需数量的 ID；不能提供 TraceID/status/At/revision/capture exemption。所有 capture 目标及内容由命令 replay 和 before 内部派生。
- `EventIDs[i]` 绑定第 i 个内部排序后的 capture。数量必须精确，少一个/多一个都失败；非法、全局旧 ID 冲突、批内大小写同 ID 均失败。零 captures 必须是零 IDs，nil 与空 slice 均可。不消费未用 ID、不随机生成 UUID、不分配 Now。
- 需要一个**新的私有封闭结果** `auditedHistoryCommand`，不能复用 `classificationCandidate` 冒充完整 after，也不能只返回通用 `validatedHistorySnapshot` 并丢掉命令关系。它只由两个入口成功后构造，内部保存 fixed profile、closed kind、值语义的 Today/Now/目标、绑定的 prior/after proof 和派生 `changed`；IDs 若保留必须自有复制。
- 结果仅提供完整 after 深复制、派生 changed 等必要查询；不暴露可变 proof/maps/slices/graph、不接受 caller 的 changed/captures、没有任意 `apply` 函数。零结果及零 prior 必须拒绝。Go package 私有封装不是防恶意同 package 代码的安全沙箱或加密签章。
- 若后续 writer 消费结果，仍须重新确认当前 snapshot 精确等于结果绑定的 prior；不能把一个合法结果拿到另一会话/更新后的 state 上提交。此阶段没有这种消费者。

## 5. 每次调用的固定流水线

两个入口共享一个很小的内部骨架，command-specific replay/capture 派生封闭在各自入口中，禁止开放 callback/flags 框架。

1. 只检查 prior proof 非零且固定 profile；此时不遍历或复制 caller 的 graph/IDs。
2. **先执行 §5.1 固定 metadata-only preflight**：对 caller before、after 分别检查顶层/嵌套结构及全部字符串原始字节；再检查输入 IDs 数量/长度。两个 graph 和输入 IDs 全部通过后，才允许对任一 caller graph 调用 Validate、FromDomain、Clone、创建索引/按长度分配切片，或扫描字符串内容。失败返回专用 typed budget/input-phase 错误及零结果，不记录输入正文。
3. 从 prior 取得自有 snapshot 副本，经 `ToDomain` 取原 task facts 和原 Supplement。将已 preflight 的 before 与原 Supplement 经 `FromDomain` 重组完整 snapshot，bounded canonical 比较必须**整份等于 prior**，不只比较目标 trace。实际 replay 使用从 prior 解码出的自有 graph，不在 caller 的 graph 上 mutate。读 proof 并不保证命令可接受：若不能经当前 bridge 精确 round-trip（例如 task ID 表示被规范化），此绑定步骤拒绝，不静默规范化 prior 或旧事件。不能据此一概拒绝小写 classification event/key；旧 Supplement 是否可保留由真实 round-trip、profile 和前缀决定。
4. 检查已受预算约束的 typed 输入语义；completion 校验 action、ID 内容和 owner；以同一个 `Today/Now` 构造新的 TaskKernel 并只执行指定命令一次。所有 purecore 错误原样或稳定 typed 包装返回，不能根据 after 放宽资格。
5. 用原 Supplement 分别重组 replay graph 与已 preflight 的 caller after，要求 whole snapshot canonical/bit-exact 相等。涵盖所有 days/chains/definitions/traces/subtasks、数组次序、可选字段和 nested notes/planned subtasks，不能只对 touched 字段打补丁。before/after 中任一合法但无关事实变化也必须拒绝。
6. settlement 从已绑定 before + replay graph 派生唯一 captures，按 §6 排序；再要求 ID 数量**精确等于**实际 capture 数，将预分配 ID 与固定 status/Now 转成内部 `historyCaptureIntent`。complete/undo 使用零 intents。调用 accepted append builder，保留其 ID/容量/checked revision 预检，不以新 gate 取代它。
7. 用 replay graph、**原样保留的其它 Supplement** 与返回分类 candidate 组合 complete after。不得在 graph 已变、capture 尚未添加的临时快照上要求最终历史检查通过。FromDomain 成功不能当作完整历史认证。
8. 对 complete after 运行 `inspectForwardClockOrdinaryHistoryVNext`（包含完整 envelope 精确 bytes/nodes/depth 检查）；再运行 `checkForwardHistorySuccessor(prior, final)`；再核对具体命令固定冻结段/捕获计数。三个条件缺一不可：**精确 replay + 完整 after inspector + 旧 history 前缀**。
9. 全部成功后才构造 closed result；`changed` 由完整 prior/final canonical 比较派生。失败返回零结果，所有输入、prior proof 和 caller after 不变。不接磁盘、无随机数、无 wall-clock 读取。

保留边界：前缀 checker 不授权任意 graph delta；完整 inspector 不证明命令；classification builder 不验证完整 after。不能为减少重复校验跳过其中任何一层。

### 5.1 Caller graph 的前置资源预算（R2）

这是拟新增入口的私有固定 gate，不改 `core.ValidateTaskGraph`、`v7.FromDomain` 或 accepted r1。威胁是任意 caller 构造的 graph，而不只是受控 Service 的 kernel 输出。原 R1 的“先 FromDomain 再 bounded Marshal”会在拒绝之前建 caller-sized 索引/AST，不能使用。

**常量与算术。** 取当前 `canonical.DefaultLimits()` 的 `N=1,000,000` 节点、`B=16,777,216` bytes、`D=128`。运行 gate 不接受 caller 自选 limits；使用 `uint64` 计数。`add(sum,x,limit)` 先检查 `sum<=limit && x<=limit-sum` 才相加；`addProduct(sum,count,cost,limit)` 先检查 `sum<=limit && count<=(limit-sum)/cost` 才乘加（cost 固定且大于零）。Go `len` 非负，先转 `uint64`；不先做 int 加法/乘法，不靠溢出后的比较。固定 shape 不递归，因此不另设更紧深度阈值；最终 envelope 仍检查 D。

**A. 先只看五个顶层 slice 的 len。** task-forest 节点计数从 5 开始（五个 array 本身），依次加 `len(Days)*17`、`len(Chains)*13`、`len(Definitions)*19`、`len(Traces)*33`、`len(Subtasks)*25`，每项用上面的 checked multiply-add，与 N 比。任何一项超限立即失败，尚不遍历该 slice，不建索引，不复制。不得仅逐项比较 len 而漏掉总量。

**B. 有界父记录 metadata pass。** 仅在 A 通过后，用下标/指针只访问字段长度和 nil 标志，不先把 nested slices 整批复制。遍历已界定顶层项，为每个 chain/trace 加 `len(NoteEntries)*11`，为每个 definition 加 `len(PlannedSubtasks)*17`；加法在进入相应子 slice 循环**之前**完成。累加所有可选字段节点成本；note.DeletedAt 的 nil 标志只能在其整个子 slice 基础成本已预扣后逐项访问。全部 top/nested 结构成本通过后，才开始 C 的字符串长度遍历。

成本直接由当前 v7 typed schema 得出，包含对象本身、对象 key 及字段 value；基础值包含必需字段和空 nested array，但不含子项或 optional 字段：

| record | 基础节点 | 每个存在的 optional 字段额外成本 |
| --- | ---: | --- |
| Day | 17 | LockedAt、三个 Review string 各 +2 |
| TaskChain | 13 | CycleMembership +14（含其 SeriesID 与 OccurrenceDate） |
| TaskDefinition | 19 | DescriptionText、SupersededAt 各 +2；SupersededByDefinitionID +4 |
| DayTrace | 33 | PinOrder、DescriptionText、ManualProgressPercent、CompletedAt、SettledAt、DraftCancellationID 各 +2；CarriedFromTraceID、ChangedToTraceID 各 +4；DraftCancelledOn +8 |
| Subtask | 25 | CarriedFromSubtaskID +4；CompletedAt、SettledAt、DraftCancellationID 各 +2 |
| TaskNoteEntry | 11 | DeletedAt +2 |
| PlannedSubtask | 17 | 无 |

计数依据：标量字段为 key+value=2；wrapped ID 字段为 key+`{rawValue:string}`=4；LocalDate 字段为 key+三标量对象=8；Membership 自身为 object+SeriesID字段+OccurrenceDate字段=13，父 key 再 +1。`draftCancellationID` 在 wire 是 uuid 标量，因此是 +2，不误算为 wrapped ID。

**C. 字符串原始字节 metadata pass。** 每个 graph 分别从 rawBytes=0 开始；只读 `len(string)`，按逻辑出现逐项 checked add 到 B，失败立即返回。不能先 TrimSpace、UTF-8/UUID 校验、大小写转换、hash、比较字符串内容、构造 error 包含内容，或通过复制/拼接取得长度。非 nil optional 只解引用字符串 header 取 len，不扫描字节。需覆盖：

- Day：ID、非 nil ReviewSummary/ReviewUnfinishedReason/ReviewTomorrowNote。
- Chain：ID、State；非 nil CycleMembership.SeriesID；每个 Note 的 ID、Body。
- Definition：ID、ChainID、Title、非 nil DescriptionText/SupersededByDefinitionID；每个 PlannedSubtask 的 ID、LineageID、Title。
- Trace：ID、ChainID、DefinitionID、Status、非 nil DescriptionText/CarriedFromTraceID/ChangedToTraceID/DraftCancellationID；每个 Note 的 ID、Body。
- Subtask：ID、LineageID、TraceID、Title、Status、非 nil CarriedFromSubtaskID/DraftCancellationID。

LocalDate、Instant、int、Difficulty 及其可选指针是固定标量，不作为字符串扫描；state/status/ID 虽是 string alias，也必须累计，不能只预算正文。B 的 structure pass 同样不能因 CycleMembership 稍后 scope 会拒绝就漏计其字段。

**D. EventIDs/输入身份。** 在任何 copy、case-fold、hash、去重 map 或 intents 分配前，只读取 slice len，从已确认有效且私有只读的 prior proof 读取其 `traces` array 的 len（不 clone/解码、不暴露底层对象），令 `E=min(uint64(priorTraceCount), N/15, B/36)`；若 `len(EventIDs)>E`，立即失败。该计数来自 sealed prior，不能由 caller before/after 的伪造长度提高；一条 trace 本命令最多 capture 一次；N/15 对齐 accepted builder 固定 eventNodes=15 的保守上限，不代替它的实际分类容量检查。随后遍历最多 E 个 ID，每项先检 `len(id)==36`，再 checked 累加逻辑 raw len≤B；不读其字符。completion 的 ChainID/TraceID 同样先以 len==36 拒绝超长值，再到第4步检查 UUID/owner。精确 capture 数和 ID 内容/冲突稍后验证；不能因长度合格提前发 seal。

**为何不误杀合法 canonical 输入。** 当前 schema 中每个逻辑出现都有独立序列化位置；A+B 算出五个 task array 子树的实际节点数，而没有计算 snapshot 顶层 keys、其它四段或 envelope。故它是完整 envelope 节点数的下界，完整合法输入≤N必然通过该结构预算。有效 UTF-8 的 canonical quote 保留非控制字符 UTF-8 bytes，转义只增不减；C 的 raw string bytes 是完整编码长度的下界。**不能拿 `6*rawBytes` 等最坏编码上界与 B 比后直接拒绝**，那会误杀可编码的合法文本。before 与 after **各自**获得 N/B 预算，不能把两份合法 graph 的总和限成一份；ID预算也独立。N/B gate 可放过最终 envelope 超限、非法 UTF-8、非法字段或引用，它不是最终验收。

**共享和工作量。** 同一个 Note slice、string 或 `*string` 被多个 owner 引用，须按每次逻辑出现计 nodes/raw bytes，禁止按地址去重；否则 Marshal 的展开量会被低估。先预扣子 slice 成本后才逐项访问，重复引用不绕过工作量上限。typed graph 无递归容器，gate 只做固定字段的 metadata 遍历、O(逻辑record数)工作与 O(1)额外状态，不构造 AST、不建等长计数表或按输入内容分配。通过后才允许原 Validator/bridge 的有界但更昂贵处理；不把此 gate 宣称为精确 RSS/耗时保证。调用者在调用期间不得并发修改输入，包括 alias header/optional pointer。

**防字段漂移。** 生产 gate 用显式 typed 字段清单，不做通用 reflection。测试端用 reflect.Type 递归枚举 TaskGraph 及上述 nested struct 的字段路径/类型，含 string aliases、pointer、slice、固定标量，和一份完整模型清单精确比对；新增/改型字段即 fail，要求同步审核预算与 schema。另逐字段设置/取消 optional，对照测试端 canonical tree 节点计数与表成本；只改测试清单不能替代预算实现更新。

**失败阶段可验证。** 私有 preflight 返回固定 typed stage/reason（如 before/after shape、raw bytes、event ID count/length），不附用户内容。测试先单测 gate，再用“同时预算超限且领域无效”的 graph 调用两个入口，必须收到 preflight 类型而不是 domain error，并得零结果/输入不变；配合测试端 Go AST 次序门禁及只读调用链审查：入口须有显式 preflight-error→立即return 的直线前置段，随后才能到达 bind/replay/FromDomain/Validate/Clone 或索引/复制 helpers；新增前置调用须纳入审查，不能让一个未检查的 helper 隐藏昂贵操作。确认两个 caller graph 和输入IDs均先 gate，不能只验before。无需生产 callback、可注入 validator 或为了观测而扩大公开 API；不把无 domain error 单独当作控制流形式证明。

## 6. Settlement 的精确合同

### 6.1 日期和图变化

完全重放当前 Go `SettleTaskDays(Today,Now)`：存在 formsDayHistory trace 且 date<Today 的日期才候选；原 day 已锁就跳过。有效 graph 已要求 trace 引用 day，因此本阶段不模仿 Swift `ensureDay` 修复缺失 day。

对于每个本次新增锁的日期：

| before trace | replay after | capture |
| --- | --- | --- |
| P | U；清 pin；settledAt/contentUpdatedAt=Now；其 pending subtasks→unfinished，settledAt/updatedAt=Now | 恰一条 U@Now |
| C | trace 完全不变，尤其真实 completedAt/contentUpdatedAt 不改为锁时间 | 恰一条 C@Now |
| 已有 U/A/R/D | trace、子任务和既有事件原样保留 | 无 |
| X，与上述 formsDayHistory trace 同日 | X 自身原样保留；该日可新增锁 | 无 |

日的 lockedAt/updatedAt=Now；不 touchChain，不改 definition、priority、notes、progress 或其它子任务。before 已锁日连 day.updatedAt 也不应因本命令更新。没有新 identity/日/trace/child；不存在重放之外的“修复”。`TraceStatus.changed` 为本 profile scope 外，保持拒绝；不与结果的 `changed` 布尔值混淆。

**X-only day 是 core 的真实 no-op**，不是 silent lock：X 不 formsDayHistory。empty day 也不因 Today 前进而锁定。同日存在 companion 才可能锁 X 的日期；不能用 after 有锁倒推 before 必然应该锁。

capture 必须使用 before 状态和本次新锁共同判定，不能复制旧 `CaptureEmptyHistory:303` 的 after U/C 谓词。尤其 source-supported open A→U（保留 A settledAt）之后首次锁日，已有 U 的 stream/latest 完整保留，不生成第二条 U。

### 6.2 固定排序和 ID 对应

保留当前 Go 的确定性政策：按 date 升序，再按规范 UUID TraceID 字符串升序排序。来源是旧 `history.go:307–312`，不是 Swift oracle。使用已规范化的 prior/replay 身份；不得以 caller after slice 顺序、map 迭代或 status 排序。

内部第 i 个 capture 使用 `EventIDs[i]`；append builder 按该次序分配 `prior.revision+i+1`，全部 capturedAt **bit-exact 等于 in.Now**，不从 completedAt 或旧 settledAt 推断。旧每流 event 保持原顺序，旧 latest key 原位更新；不能重新插入同 key。

Swift 的 dates 与 capture IDs 为 Set，traces 来自 Dictionary，未排序（Engine:3018–3023,3039,3046–3064）；UUID 为构造默认值。静态证据只支持目标集合/数量/status/Now/连续新增 revision，不支持每 trace 的固定 revision 或 UUID。未来 Swift 差分需比较同批次合法置换后的语义关系，仍逐项核对原历史；不能删除真实差异或声称 Go 排序是 Swift 字节等价。本阶段不运行 oracle。

### 6.3 空 captures 不等于 no-op

已有 U/A/R/D 的 open day 可新增锁却零 captures；mixed X 亦然。这是 graph mutation，`changed=true`，不消费 event ID，但未来 Service 仍必须分配严格新 Now 并提交。仅完整 prior/final 相等才是 no-op。

## 7. Complete / undo 的精确合同

- 必须 today 合法且等于目标 date、day 未锁；complete 要求 P 且没有 pending/unfinished child，undo 要求 C。无关目标、空 ID、owner 不匹配、错 action、过去/未来/锁日均失败；相同状态再次请求不是成功 no-op。
- 重放 `MarkCompleted` 或 `UndoCompleted`。complete 写 target.status=C、completedAt=Now、contentUpdatedAt=Now、pin=nil、owning chain.updatedAt=Now；undo 写 P、completedAt=nil、contentUpdatedAt=Now、pin=nil、同 chain.updatedAt=Now。其他所有事实由精确 replay 冻结，不能顺手补 child、清 settledAt、改日锁或 definitions。
- **整个 classifications canonical 完全不变**，不仅 revision 不变；旧 A stream/latest 原样保留。P/latest A → C/latest A → P/latest A 必须通过，不能把 latest 改成当前状态，不能删除 A。
- 后续 settlement 对这个 C 追加 C@lockNow，保留最后一次真实 complete 的 completedAt=completeNow。locked C 的 completedAt 篡改即使仍是合法对象 clock，也必须由 replay 或完整 inspector 拒绝。
- 通用 inspector 不要求所有 C.contentUpdatedAt==completedAt；**本次 complete 的精确输出**要求二者都是本次 Now。两种权威不可互换。

现有静态跨引擎差异：Swift touchChain 为 `max(oldUpdatedAt,now)`，Go 会先拒绝 now<chain.UpdatedAt，再赋 now。新 helper 重放当前 Go，不趁本阶段修改 core；在 trace clock≤now<chain clock 的输入上不承诺 Swift 对等。这不是 r1 helper 的缺陷。

最小 fixture 条件：同一合法未锁当日图，目标 trace.contentUpdatedAt=1、owning chain.updatedAt=10、Now=5；complete 使用无 open children 的 P，undo 使用 completedAt=contentUpdatedAt=1 的 C，created/day clocks 不晚于 1。Swift 可保留 chain=10 而改 trace=5 仍是源码推导，**未运行 Swift，不称为跨引擎 red/green**。

整合阶段已对正式 `03a18cca` 做 Go 单侧外置 overlay 观测，证据为 `/workspace/shared/noonmark-completion-clock-observation-20261106/observation.json`：`go test -p=1 -race -overlay <external overlay.json> ./core -run ^TestObserveCompletionOlderThanChainClock$ -count=1 -v`，exit 0；complete/undo 均得到 ErrInvalidClock，graph bit-exact 不变。fixture 先以 CreatePoolTask、ScheduleFromPool（undo 再 MarkCompleted）Now=1 合成，再**显式将 chain.UpdatedAt 置10**并经 NewTaskKernel 验证；这不是 Service 历史或正常 writer 可达性证明。证据内记录 probe/overlay/log hashes；设计作者只读取该证据，没有重跑。该观测不是新 typed helper 的测试、Swift 差分或产品修复。

未来全 persisted 严格前进的 Service clock 排除此输入；完整复刻仍须在后续独立来源差分专题实证及处置，不能永久宣称 complete/undo 全域 Swift 对等。

## 8. 时钟、no-op 与 MaxFloat 的层级

**本阶段**只接受显式 finite Now，服从 core 局部 clock 和最终 fixed profile 校验；不读墙钟、不调用 nextUp，不额外规定所有命令 Now 严格大于所有 prior clocks。core 允许数值相等 Now（含 +0/-0）；赋值关系、replay、前缀用 bits 精确比较，先后/max 用数值比较。

- complete/undo 在合法相等 Now、甚至 MaxFloat64 的纯输入上可成功；它们仍是 graph mutation，不得误称 Service 可继续写。
- settlement 的 true no-op 可以使用 finite MaxFloat64，零 IDs、zero revision delta、完整 snapshot 不变。NaN/Inf 即使“看似 no-op”也由明确输入校验/core 拒绝。
- settlement 用 MaxFloat64 执行真实 mutation，在 core 和 profile 都通过时可作为纯来源正例；未来 Service 若 frontier 已为 MaxFloat64 则无法严格前进，必须在提交前失败。
- 不根据其它 chain 的更晚、未受影响 clock 给纯 helper 添跨链全局严格递增限制；但新 captures 与所有旧 events 的全局非倒退要求、同日 lock/profile 约束仍由完整 inspector 强制执行。core 成功而 profile 失败时整次 audit 失败，例如 `[A@10]` 后 silent lock@5；不“修复”时间。

**未来另阶段**须补全统一 collector（所有 persisted facts，含全部 events/latest、terminal、planned subtask、note tombstone、preferences 等），写锁内从该 frontier 分配一次 Now。不可认为当前 `Service.persistedClocks` 已覆盖这些事实。

未来 reconcile 应先用 frontier 做不发布的 task-graph probe；graph 真正无变化时再以零 IDs 验证 no-op、verifyPrefix，避免 Next/UUID；若 graph 变化（包括零 capture 的 silent lock），再取严格 Now、重新 replay、分配恰需 IDs、审计、一次 commit。不能拿 probe 的 graph 或 clock直接发布。Today 仅在成功后发布；这不是本阶段接线承诺。

## 9. 实施与验证分解

以下全为**拟实施测试**，本轮未运行。正例优先用 core Create/Schedule/Complete/Undo/Return/Carryover/Abandon/Reactivate/Settle 合成，再独立投影预期 captures；不能让待测 helper 自己生成自己的 oracle。

### 9.1 最低测试矩阵

| 组 | 必须覆盖 |
| --- | --- |
| 正常来源 | P→U、C 新锁；A→P→C→lock；complete/undo 多轮保留多条 A；有多日期多 chain 的来源构造 |
| 已有历史 | open A→past U→lock：无第二 U、settledAt 保留旧 A；open A/R/D silent lock；既有 locked U/C/A/R/D 完全不动 |
| X/空日期 | X-only past date 真 no-op；X+同日 companion 新锁，X 无 capture且原样；空日、today、future 均不锁 |
| 多 capture | 反向数组布局、多日期多 trace、已有 A 流、新流混合；精确 date/ID→ID/revision 对应；重复运行 bytes 相同；同批 Now bits 全等 |
| 缺/多 capture | 缺 ID、多 ID、旧/重复/大小写冲突 ID、非法 ID；把已有 U 算为新 capture；错 status/trace/clock 均不能通过结果组合检验 |
| before 绑定 | prior A 与另一合法 before B，目标相同但无关 chain/definition/note/child 不同；before数组换序；零 proof；不能只比 target |
| 恶意 after | 合法 replay 上另改无关 chain state/clock、notes/tombstone、definition version、priority、manual progress、child status、增删对象、旧锁或数组；必须拒绝 |
| target/action 欺骗 | complete目标x却提交y的after；ChainID与TraceID不匹配；换Today/Now/Action；dangling/零 ID；拒绝历史/未来/锁日/非P或非C/open children |
| 完整历史围栏 | 两端各自通过 inspector 仍篡改旧 event ID/clock/revision或删旧A并重编号；必须 prefix/replay拒绝；C completedAt实际锚错误拒绝 |
| supplement/scope | 原分类之外 sections逐字保留；非 neutral preferences、非空catalog/ideas/series、CycleMembership、changed拒绝，不以empty supplement洗白 |
| signed zero | +0与-0数值相等可走合法core动作，但赋值/旧锁/旧events bits保留；只翻目标completedAt或event.Now位型即拒绝；不要把inspector的替代来源解释当本次命令 |
| 日期端点 | Today=min/max合法；min没有更早合法日期；max date不能新锁/虚构10000年；max日期complete/undo可在Today=max；X-only=max不锁；无效闰日/零Today拒绝 |
| clocks/no-op | 负有限、小数、子正规、相等、MaxFloat；NaN/Inf拒绝；真no-op零IDs、不用nextUp；silent lock零captures却changed；局部core成功/profile拒绝保持拒绝 |
| 跨链事实 | 无关较新clock保留，不擅触碰/不伪称Service；未来严格frontier与本helper区分；所有同日companion包含在整体replay |
| 原子纯输入 | 在输入校验、replay、append、完整after、prefix各阶段失败后 prior/before/after/IDs不变；result零值不能取snapshot |
| alias/concurrency | 成功后改caller before/after/IDs、修改每次返回snapshot copy，不污染proof或下一次结果；共享immutable prior并发运行不同命令、反复copy/append，race无写；不承诺caller并发写入输入也安全 |
| 前置顶层/嵌套预算 | before与after各自：五顶层总成本超N；一条chain/trace超量notes；definition超量planned；多个父项各自不大但总和超限；在嵌套内容访问/索引/Validate前拒绝 |
| 前置字符串预算 | 每个列出的string/alias/optional string/optional ID独立超B；单项各不超B但逻辑总量超B；共享slice、共享正文、共享*string仍按出现计；非法UTF-8内容不在gate先扫描 |
| 预算临界与溢出 | 私有add/addProduct在limit-1/limit/limit+1、零count、uint64极值的无大分配测试；节点表及optional增量对账；rawBytes=B gate可通过但完整envelope仍可拒绝；B+1在扫描前拒绝 |
| IDs/阶段 | EventIDs count>E未遍历即拒绝；空/35/37/超长ID先长度拒绝，重复36字节到后续builder拒绝；两个graph分别domain-invalid且budget-invalid时必须预算阶段失败；result零/输入不变 |
| 合法输入不误杀 | 由core合成的有效snapshot，经完整inspector及ToDomain后两个graph均通过gate；有效UTF-8/转义文本、共享逻辑叶片、接近真实envelope边界的来源正例；不把metadata边界通过说成完整snapshot通过 |
| 字段清单与最终边界 | test-only反射完整模型清单防新增string/slice/pointer漏计；全部字段/optional表对账；最终canonical bytes/nodes/depth及builder checked revision/capacity仍执行；无指数级历史replay |
| ID大小写对照 | 小写task rawValue已通过reader但精确binding拒绝且prior不变；只有旧classification event.id或key/traceID小写时，按真实round-trip/profile接受并保留旧bytes，不一概拒绝或改写 |
| 未接线回归 | 旧ValidateEmptyHistory仍拒绝A、多事件/openU；旧Capture/Service/store/v2/codec callback调用点不变；已有return/carryover/manual/subtask行为不扩容 |

测试分层须诚实：API 的 after 参数是 task graph，不携带调用者自选 classifications；旧事件篡改/漏 capture 的完整端点负例由既有 inspector/prefix 层及显式测试组合覆盖，命令入口另核对结果完整分类和必须调用三重校验，不能声称一个根本未传入 helper 的恶意字段被该入口直接拦截。不得为注入负例而开放生产 callback 或 caller captures。

额外固定断言：settlement 冻结 chains/definitions，只有精确 replay 许可的 days/traces/subtasks改变；completion 冻结 days/definitions/subtasks及除目标、owner之外所有图事实；两个入口都冻结非分类 supplement，complete/undo再冻结整个分类。这些固定围栏只加强具体命令，不进入 generic exemptions。

### 9.2 实施切片与准入

1. **设计复核**：R2仅针对唯一R1输入预算及对应矩阵只读复核；确认所有caller graph/IDs先gate。原API、排序、owner binding、X-only、clock及来源差异保持；正式已提交的r1树仍只读。
2. **独立实现**：待 R2 设计复核通过并获准后，在隔离源码工作区只新增 command helper/测试，不复制toolchain/cache；先写 source-positive 与 malicious-delta red tests，再实现固定流水线。不要重新编写 r1 inspector/NFA。
3. **作者验证**：固定 Go1.27.3、显式外置 BUILD/GOTMPDIR/log、已有cache，GOPROXY/GOSUMDB=off、低并行。focused `history command` + old history/aggregate，taskapp全测、race、vet；保存command/exit及冻结hash。未通过不进入整合。
4. **独立审查**：从冻结实现另作来源正例和恶意delta测试，不复制作者oracle；重点审完整after/prefix不可短路、同批排序、openU、MaxFloat、zero proof/aliasing。审查未通过保持未接线。
5. **整合门禁**：只在独立验证通过且整合阶段同意后整合新增文件与小范围文档；在精确整合树跑既有 `ports/mygo/scripts/check`、项目要求的cross/build/CLI回归。源码静态diff证明Service/store/旧callback未接线。没有Swift/非Linux实机就明确未验证。

纯 helper 的 race/单测不能代替 future Service 双handle、reservation fault、reopen、Today发布、Snapshot竞态或UI症状验证；本阶段不安排这些磁盘测试冒充writer完成。

## 10. 风险、回滚与下一边界

- **数据完整性（P）**：after-only capture 会重复 U；只比 touched字段会放过无关恶意delta；只看inspector会接受错误命令。以三重校验和来源正反例阻断。
- **假权威（P）**：proof/result名字不能被解释为授权、可导入或已持久化。typed结果留private且无Service消费者。
- **来源误报（P）**：Go稳定排序不等于Swift Set顺序；Go较旧chain clock拒绝与Swift max存在静态差异。显式保留差异，不造oracle、不趁机修core。
- **资源/别名（P）**：先对两个caller graph/IDs做固定metadata-only节点/raw-byte预算，再进入领域验证、copies/AST；最终bounded canonical验证与checked容量仍保留；并发只读输入要求写清。实现后须运行证据，设计本身不宣称安全已验收。
- **灰度/监控**：本阶段只有隔离合成输入，无真实会话、流量或监控。日志仅typed stage/reason、数量、耗时，不记任务正文或真实路径。
- **回滚**：未接线文件可整体撤下；保留r1 accepted文件与既有会话不动，不改旧事件/锁/revision，不以清缓存或删会话解决失败。
- **后续独立阶段**：typed Abandon/Reactivate/current return/defer/continue接纳已有流；全persisted clock；全新development v3 clean cut；writer/store fault/concurrency/reopen；最后UI。旧v2不迁移、不覆盖、不删除、不原地续写。以上均不是本阶段完成项。

本阶段验收用语仅为“未接线 typed settlement/complete/undo 精确审计通过隔离验证”。不能称 Abandon/Reactivate Service、v3、全跨平台复刻或Swift全量对等已完成。
