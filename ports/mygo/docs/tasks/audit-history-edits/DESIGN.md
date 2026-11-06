> 阶段归档：保留原设计或隔离副本验证边界，正式整合状态见 README。

# 标题与手动进度：封闭历史编辑审计设计

2026-11-06。仅源码研究与设计，未实施、编译、执行测试、接线或生成运行 fixture。正式 Go HEAD `2a808a3c4fe788c69d8f47cf21e1d686860a19ff`，工作树 clean；上游 `43e6eefaae3a04be89cab3e234d470111afd80ef`。精确输入文件 SHA-256 见 SOURCE-SHA256.tsv。风险 P，保护历史与编辑能力，不宣称 Swift oracle 或跨平台已通过。

## 1. 范围和最小落点

新增 package-private `history_edit_command_vnext.go`、`history_edit_command_vnext_budget.go` 及专用测试。两个 audit 入口：title（闭合 Rename/Input action、chain/trace owner route）与 manual progress。输入为 sealed `validatedHistorySnapshot`、caller before/after `core.TaskGraph` 与强类型参数。没有 Service/store/UI/v2/r1/core 改动，没有 allocator、clock collector、迁移或磁盘写入。除已明确批准的共用结果 valid() 三种 kind 白名单注册外，旧函数身体冻结；见第 8 节。

只补纯证明，不替换旧 `captureTitleHistory`/`captureManualProgressHistory`，不改旧 ValidateEmptyHistory。未来全部 consumer typed 对等才可切换，见 inventory REPORT.md；创建、排期、子任务、回池、延期/延续等不因本批具备写权限。future P 带 A → X 的组合仍以 APPENDIX-SWIFT-FUTURE-CANCEL.md 的证据边界处理，不删 A，不扩大 inspector。clock 背景设计不授权 +0 floor，未证明旧 A/U collector 缺陷。

## 2. 原始行为与接口

Swift `Sources/NoonmarkCore/NoonmarkEngine.swift`：renameTaskTitle 1216–1229、saveTaskTitleInput 1234–1247、updateTaskTitle 1249–1320；normalizeTitle 3556–3568；manual 2842–2864、hasSubtasks 3937–3948、progressFloor 4004–4019。Go 核心 `domain-core/core/title.go`、`manual_progress.go`、`progress.go` 是本批精确 replay 对象。Swift Foundation trim 与 Go Unicode trim 的 Apple oracle 仍未跑，不能将本批称全 Unicode 平台对等。

建议私有类型：
- `titleHistoryAction`：`titleHistoryRename`、`titleHistorySaveInput` 两项，不复用旧 Service titleMode。
- `titleHistoryRoute`：`titleHistoryChain`、`titleHistoryTrace`；trace route 仅准 titleHistorySaveInput，对应现 SaveTraceTitleInput[Acknowledged]，禁止 rename+trace 的不存在组合。
- `titleHistoryInput`：Action、Route、ChainID、TraceID、Title string、Today、Now、NewDefinitionID。chain route 必须 TraceID 空；trace route 必须提供选中 trace 和显式 ChainID 绑定。没有 optional 指针、map、slice、callback 或 caller exemption。
- `manualProgressHistoryInput`：ChainID、TraceID、Percent int、Today、Now，字段与 core.SetManualProgressInput 对齐。
- `auditTitleHistoryVNext(prior,before,after,in)`、`auditManualProgressHistoryVNext(prior,before,after,in)` 返回 closed result,error；所有错误返回零结果。不得提供直接导入 arbitrary AST/result 的 constructor。

Title trace route 是 port capability fence，不是 Swift engine 原参数：budget 与 binding 后，验证 action/route/date、consumed IDs；找不到 trace 返回 ErrNotFound，chain 不匹配返回 historyCommandOwner；date<Today 返回 ErrImmutableHistory；非 pending 返回 ErrInvalidTransition。之后 core 原样执行，包含锁日检查。chain route 不额外要求 trace，这保留池草稿与 R 可编辑能力。不得仅看同链另一条 pending 而绕开选中历史 owner。

标题核心分支（顺序保留）：Today/UTF-8 合法 → CRLF/CR 转 LF、TrimSpace（空标题允许）→ chain 存在/active → formsDayHistory traces 非空时须有 P 或 R → 任何历史 pending 或 pending 锁日均拒绝 → 找 current definition → Unicode canonical equivalent 即 no-op，保留已存原始 bytes → 实际变化才查 Now finite 且不早于 current.contentUpdatedAt → 决定 reuse/version。
- 无 day history：Rename/Input 都 reuse。
- 有 history、Input 且 current 未被非 P 历史 trace 引用：reuse，只改 current title/contentUpdatedAt。
- 其余变化：新版本，查 sequence 溢出、NewDefinitionID 合法且未占用；pending trace clock 不得领先 Now；旧 current 只改 contentUpdatedAt/supersededAt/supersededByDefinitionID；新定义 sequence=max+1、title 为 normalized input，复制 description/planned children；pending references 和 clocks 转到新定义。其它历史 traces 引用/content 保留。
- chain.updatedAt=max(old,Now)，不压低 note 带来的更新时钟。
- unused NewDefinitionID 在 no-op/reuse **不校验 UUID、不查冲突、不预留**，包括空、短字符串、已用 ID。在 metadata 总字节范围内任何 unused 字节均可接受。不能用强制长度36偷偷收窄 core。

Manual 顺序：Today 合法 → trace 存在 → chain owner → 所在日未锁 → date==Today 且 P → 全链历史 presentable children（不是仅当前 trace children）阻断 → next=max(历史 floor,clamp(input,0,100)) → 已有 pointer 非 nil 且等于 next 即 no-op → 变化才查 Now finite/不早于 trace.contentUpdatedAt → 只改 trace manualProgressPercent/contentUpdatedAt。nil→0 是变化；100 不完成父项、不触碰 chain clock；取消草稿 child 不构成阻断。floor 来自更早日期或同日较小 continuationSeq 的历史 rawProgress 最大值，不由输入覆盖。

## 3. 资源边界：所有昂贵工作之前

两个入口第一条 prior.valid（只验 seal），接着三条 straight-line fail-closed preflight：
1. `historyCommandGraphBudget(before,historyCommandBefore)`。
2. `historyCommandGraphBudget(after,historyCommandAfter)`。
3. 新 `historyCommandTitleInputBudget(in)` / `historyCommandManualProgressInputBudget(in)`。
第四条 gate 后立即 `historyCommandBind(prior,before)`，与旧 AST 顺序相同。此前不得 FromDomain/ToDomain/Validate/Clone/索引/string content scan/utf8/norm/strings.TrimSpace/UUID parse/申请 caller-sized 容器。graph budget 的 raw pass 只读字符串 header len，允许。

复用实际 `historyCommandBudget`、`historyCommandAddProduct`、`historyCheckedAdd`、`historyCommandBudgetFail`、canonical.DefaultLimits（MaxBytes 16MiB、MaxNodes 1,000,000）。新增独立 budget helper，不修改旧 budget 函数身体。新 input 错误 phase 可增加 `historyCommandInputFields` 枚举，reason 复用 RawBytes/IDLength；这样 Title 超限不误报 event IDs。类型固定节点数量，不含可变集合，先固定字段形状后累计所有字符串逻辑使用 raw bytes：Title+ChainID+TraceID+NewDefinitionID。每个字段各算一次，即使 alias 同一底层字符串；累加 overflow-safe，总和 <=MaxBytes。Manual ChainID+TraceID 同理。可对必消费 ChainID/TraceID 作 len=36 metadata 检查；chain route 的 TraceID 空形状检查可置预算后语义层。NewDefinitionID 不作定长检查。标题长度不是字符数，超长 whitespace 也须在 trim 前拒绝。

两个 graphs 各按现有预算独立 <=limits，input 自己 <=limits，不把 before+after+input 三份相加再限制16MiB（会错误拒近 envelope 合法输入）。最终完整 canonical envelope 仍由 bridge/inspector 限制，这里不是 output capacity 保证。不得用 worst-case 6x JSON escaping 上界拒绝实际合法 near-envelope；实际 escaping 超限由后层拒绝。resource failure 可先于领域错误，是显式 bounded adapter 合同；不是 core UUID 新语义。

## 4. 精确审计流水线与错误

在预算后 `historyCommandBind`：从 sealed prior owned copy decode，用 caller before FromDomain 精确匹配，并确认 sealed roundtrip 未改变 task rawValue。应用封闭 route/shape guard；`core.NewTaskKernel(owned)`；只调用 RenameTaskTitle/SaveTaskTitleInput/SetManualProgress 原函数；禁止重写领域算法。core 原错误原样返回，包括 InvalidInput/NotFound/ChainAbandoned/InvalidTransition/ImmutableHistory/InvalidClock 及 graph validation 错误。

`historyCommandMatch(replay,after,extra)` 对整个 after 精确比对，防止附带未请求修改；不只验证目标。`historyCommandFinish(prior,replay,extra,nil)` 必须原样经过 appendEmptyHistoryEvents(nil) → classificationsCopy → FromDomain → 完整 inspectForwardClockOrdinaryHistoryVNext → checkForwardHistorySuccessor → fixed supplement fence 与 exact capture count。两个命令永远零 capture，没有 EventID 输入。不能因为 no-op 跳过任何步骤。之后独立 fixed fences，再构建结果。

seal 不合法→historyProof；budget→typed budget error；binding/after mismatch→historyCommandBinding/Replay；非法 action/route/ID 形状→historyCommandInput；owner→historyCommandOwner；freeze→historyCommandFence；finish 的 inspector/prefix/count 错误原样。API 不接外部 classifications candidate，故分类篡改负例应放 builder/finish/prefix 单测，不谎称经 audit 参数直接注入。整图 raw IDs 不能大小写 normalization。合法原事件大小写键、顺序、revision、capturedAt bits 全保留。

## 5. 独立固定 fences（不可传 allowlist）

Title：days/subtasks/classifications 精确不变；Finish 另守 preferences/ideas/taskCycleSeries。chains 数量/order/ID 不变，非目标链全部字节相同；目标链除 updatedAt 外全部相同，updatedAt 应为 max(old,Now)，no-op 原样。traces 数量/order/ID 不变，所有非目标链与所有非 pending trace 完整冻结。目标 pending trace 仅在实际新增定义时允许 definitionID/contentUpdatedAt 按确定 successor/Now 改动；reuse/no-op 整条冻结。

definitions 保持原有顺序/identity，不删不重排；非目标 current 完整冻结。分支由 owned/replay 实际确定的 current 和新 ID 数量得到，不由 caller 提供豁免。no-op 全段冻结；reuse 数量相同、仅 current title/contentUpdatedAt 可变且由 exact replay 固定；version 恰新增1且为 NewDefinitionID、chain一致、maxSequence+1、新 CreatedAt/ContentUpdatedAt exact Now、无 superseded metadata，复制 description 与 plannedSubtasks 的 presence/nil 和字节。旧 current 仅上述三 supersession metadata 改变；已 superseded old defs 全冻。包括 non-P历史引用 current 的情况，不能把该定义正文开放可改。restored current 有旧 successor witness 时不得额外拒绝，应只替换为真实新 successor（现 core 允许）。

Manual：除 traces 外所有顶层 key/值精确冻结，key 集也固定；traces 数量/order/ID 冻结，非目标 trace 整条冻结；目标只允许 manualProgressPercent/contentUpdatedAt 两字段，projection copy 要保留其余字段与 presence；这两值已由 replay 确定，不能由 caller unchecked copy 代替核心证明。zero capture/count 与分类精确不变单独断言。

Fence projection 新建私有 object，不修改 prior map；不把浅 projection 暴露给 caller。只能在 bounded graphs 后建立索引；偏好按 canonical 数组位置+ID 核对数量顺序再定位，防止“map 看起来一致但原顺序丢失”。

## 6. no-op、时钟、身份与无别名

不能照抄 lifecycle/completion 的 unconditional Now.Finite。Title canonical-equivalent、Manual 已有 next no-op 不消费 Now，允许 finite 更早、±0、MaxFloat、NaN/Inf raw bits。护栏仍先执行，历史/锁日/wrong owner 不因 title/progress 相同获准。真实变化允许 equal Now（core局部 clock nondecreasing）；无权新增全局严格后继或 Now>+0。实际 Service 真变化必须另按将来批准的全局严格后继分配，本纯 helper 不给该授权。

结果只保存私有 scalar request identity 与 owned sealed endpoints。建议字段最小新增 titleAction/titleRoute/requestedTitle/requestedNewDefinitionID/requestedPercent，chain/trace/today/now 沿现有 scalar，kind 区分三动作。Go string 不可变，可按值保留，不能保存 input pointer。无需保存 acceptedTitle/percent、耗用 ID 集或 replay graph：从 final owned snapshot 导出，避免双事实源。

明确 requested* 与 now 是输入身份，**不是已消费 clock/ID**。changed 只能取整个 prior/final 精确差；定义是否创建通过 endpoints 差值确定，不从非空 NewDefinitionID推断。no-op result 带 NaN Now 也不是可用 mutation timestamp。未来 writer 必须绑定 actual request/session/head、检查 changed 和实际 branch，不用 proof.now 盲目推进 frontier。当前不提供 public ack，snapshotCopy 不是提交凭证。

不得泄漏 private proof、map/slice/pointer；snapshotCopy 必须深复制。审计期间 caller 不得并发 mutation/shared leaves；成功或失败后修改 before/after（包括 note、planned children、optional）及已返回 snapshot copy，不得改变 prior/result；反向修改第二份 copy 亦然。失败结果无有效 proof。

## 7. Fixture、oracle 与验证边界

测试矩阵见 TEST-MATRIX.md。生成两类明确标注 fixture：
- 命令合成 fixture：内存 CreatePoolTask→ScheduleFromPool→新 abandon/reactivate/settlement 已有纯 proof 管线组成 [A]→P、[A,A]→P、[A,U]、[U,A,U]。每步 core执行、审计 sealed prior、完整 inspector、prefix 成功后才给下步。trace 是历史 U 时 title 没有 P/R 仍应拒绝；manual 需真正当天 P，可在别链拥有历史流，或新 continuation 命令生成合法 P。不得靠改状态冒称可达。未来 R/D/C 派生若尚无 typed 适配，只称 core+independent fixture，不称 Service 路径。
- 纯参数/profile fixture：equal Now、negative epoch、±0、NaN no-op输入、restored witness、maxSequence、near-budget graph；每个 prior 仍通过当前 full inspector。sealed-valid 不等于 Service会产生。不得混同 current Service（仍旧 reader）已能写 [A,A]。

expected 不只调用同一 replay算法：独立固定字段 oracle 检查 title bytes/version count/old content/new definition copy/target exact delta/classification bytes 与 count；manual 独立 floor/clamp case表，不调用 ProgressProjection 算期望。进一步 Apple Swift 合成 oracle（以后授权环境）比较 normalized bytes、version counts、history references、manual floor，UUID由关系映射不是随机值硬比，instant以reference seconds bits对齐。此轮只列计划，没有运行 Swift。

## 8. AST 门禁、最小改动与冻结决策

现 `TestHistoryCommandBudgetASTOrderGate`（history_command_vnext_budget_test.go:424）只 parse command/lifecycle 文件，按四 if + immediate bind 检查，必须加 parse 新 edit 文件、新两入口、对应 metadata helper。保持旧入口断言不降级；新增 helper metadata allowlist只允许 len/string/uint64/现预算算术，不许 UTF8/norm/Validate/Clone/内容扫描或 caller-sized make。新增独立 edit AST gate验证 no unconditional Now.Finite、三固定 core命令、finish(nil)、只有两 audit 构造新 proof、无 Service/store/allocator/clock/network引用。旧 source frozen hash 门禁需用 baseline逐文件/逐函数 body对照，不只 grep无调用。

闭合结果已由本次委派负责人明确选定最小注册例外：共用 `auditedHistoryCommand`，追加三 kind（historyCommandRename、historyCommandTitleInput、historyCommandManualProgress）与第6节私有 metadata。唯一旧函数身体例外为 `auditedHistoryCommand.valid()` switch 增加这三种 kind；metadata声明与构造入口注释同步更新（当前四入口，增加两入口后为六入口）。不新增第二个等价闭合结果类型。旧 auditSettlement/auditCompletion/auditAbandon/auditReactivate、core、finish/bind/match/budget、Service/store/v2/r1函数体字节冻结。新预算仅在新文件。AST gate可增入口覆盖，不弱化旧断言；更新 closed result shape/unknown-kind测试，不能把任意非零 kind视为有效。

实施 allowlist：两个新生产文件、新测试文件；history_command_vnext.go仅 enum/proof字段/构造注释/valid case注册；既有测试文件仅增加文件解析与entry覆盖/新kind断言。逐函数冻结 manifest 在实施前生成 body哈希，实施后逐项比较；整文件SHA以本包 SOURCE-SHA256.tsv为原始证据，不能让新增声明掩盖旧身体变化。

## 9. 风险、退出与回滚

主要风险：unused参数被过度校验、no-op时钟不兼容、trace route越权、历史定义冻结过宽、复制规划子任务丢 nil/presence、预算顺序漂移、把纯证明当writer权限。审查者逐项核第2–8节及矩阵。禁止扩大profile、修core、偷加零floor或优化删除完整inspector/prefix。

实施获准后仅隔离development测试；先新单元/独立review，再现有全部fast gates（不得减少），未来真实Service/UI症状与各平台另批。该批仅源码纯函数，不改变持久文件，无迁移回滚；撤销新增文件/声明/门禁注册即可，保留旧路线。发生scope扩展或已有测试语义冲突停止接线并报告内部审查，不删用户资料、不将v3写回v2。Linux测试不代表mac/win/arm64或Swift视觉验收。
