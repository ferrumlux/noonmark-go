归档说明：以下为冻结设计或独立阶段记录。状态和路径指当时阶段；原文 SHA-256 为 `487f317195ed105f308dd9da6f71def2a094b9336e2901af81669f1293b7005e`。整合进度以本目录 README 为准。

# 隔离 lifecycle history 审计设计（待独立复核）

日期：2026-11-06。任务：只设计 typed AbandonChain／ReactivateAbandonedChain 审计，不实施、不接线。

## 结论与来源澄清

1. Abandon 精确重放并内部派生恰一条 A capture；Reactivate 必须按完整来源语义派生 **当前／未来未锁 A→P：零 capture；过去或已锁 A→U：恰一条 U capture**。所有分支保留旧 A。
2. 初始“所有 Reactivate 零 capture”是整合任务的临时假设，整合任务随后明确要求以完整 Swift Engine／Store语义为准；它不是授权范围限制。本设计已按实际来源纠正，不修改 core 或 fixed profile。
3. 当前 core与accepted profile在上述capture合同一致：Go core无分类，注释要求aggregate对U capture；Swift Engine:2568–2570同样如此；现profile已支持 `[A,U]`。不存在需要放宽profile的已证实冲突。任何core成功但完整profile拒绝的具体输入仍保持拒绝并报告。
4. 最小建议实施批次为两个未接线typed lifecycle helpers、预算及闭合结果最小扩充、独立来源正反例；先独立设计复核，后隔离实施与审查，不进入Service/store/UI。

## 1. 取证与范围

正式树为 `/workspace/scratch/586e77bd59a6/noonmark-mygo-work/noonmark-go`，最初检查 HEAD 为 `03a18cca51766e1d6b501e5e015ce96d0be5a7c7`，随后已只读确认提交为 `3ea3efeaee2a0085576ca1aadd0d3082e643a558`，包含七个 accepted `history_command*.go` 与设计文档。整合任务提供 tree `b7097068ebf44c167b6df8ac9162261946ea3bfb`；实施前重新冻结精确 HEAD／文件 hashes。

上游只读 `../noonmark-reference` @ `43e6eef`。已读其 AGENTS、当前 port AGENTS／DESIGN、ADR-0004/0020/0022/0027/0037、现有 typed helpers／预算／NFA／prefix／core lifecycle／相关测试。当前隔离授权取代上游另一台主机的历史路径要求；安全身份隔离仍保留。

Go 路径以下均相对 `ports/mygo/`。关键实际代码：
- `domain-core/core/history_chain.go:8–155`：输入、路由、状态／时钟和原地 reactivation。
- `internal/taskapp/history_command_vnext.go`：两个入口、sealed result、bind/match/finish。
- `history_command_vnext_budget.go`：两个 graph 的固定 metadata-only 预算。
- `history_vnext.go:247–260`：U 必须 tail U；`history_vnext_machine.go` 的 U event 消费分支才提供 A→U，epsilon closure 不提供 A→U。
- `history_vnext_builder.go`：append-only、完整 prefix 与 day-lock 检查。
- `domain-codec/v7/domain.go:22–60`：FromDomain 会 Validate、Marshal、反射投影和 clone，不可放预算之前。
- Swift `Sources/NoonmarkCore/NoonmarkEngine.swift:2513–2572`：Abandon capture A；Reactivate 同一 ID、U 才 capture。
- Swift `TaskClassification.swift:4022–4056`：append event、更新latest、revision+1。`App/NoonmarkMacApp/NoonmarkStore+TaskMutations.swift:1009–1051`均走candidate／moment；`NoonmarkStore+NaturalDayPersistenceImport.swift:886–971`先mutation、save，再发布candidate。自动分类job plan不是删除历史。这里仅借其完整capture语义，不声称Go实现Store协议。

已读消费者盘点及 `APPENDIX-SWIFT-FUTURE-CANCEL.md`；它们的 SHA 记录于 SOURCES.sha256。未运行 Go、Swift、build、test、App 或网络操作；没有生产树写入。这里的结果是源码推导，不是通过验证的实现、Swift parity、当前用户缺陷或 Service 可达性。

## 2. 不可变安全边界

固定复用 `forwardClockOrdinaryHistoryVNext`；不新增宽松 seal／fallback／回调／caller capture／validator bypass／generic field exemptions。普通空 catalog、neutral preferences、无 ideas／recurrence／changed 等现有限域不变。

prior 只能为已有 inspector 构造的 sealed `validatedHistorySnapshot`；零值拒绝。成功结果保留自有 prior 与完整 final proof，无可变 AST、index、supplement 或 graph 暴露。两端必须精确 whole-graph replay、原 supplement、完整 after inspector、旧事件 prefix 和原锁保留，缺一不可。proof 仅证明某个输入在此 fixed profile 下的单条精确边，不授予 write、session/head authority、global mutation frontier、整图历史可达性或 Swift 等价。

旧 `ValidateEmptyHistory`／Capture／Service／store／codec callbacks／UI 一律不动。消费者盘点指出标题、子任务、手动进度、return、carryover、普通新建和 reconcile 都仍可能被整个 prior 的 A 挡住；不能用本批 helper 提前接线、删除旧功能或宣称 v3 对等。

## 3. 拟议精确 API

全部 package-private，返回现有闭合结果类型；以下是规格，不是新增源码：

```go
type abandonHistoryInput struct {
    ChainID core.TaskChainID
    TraceID core.DayTraceID // 用户请求身份，未必是实际被废弃的 trace
    Now core.Instant
    EventID string // 唯一新 A 的身份，不允许传 status/trace/capturedAt
}
type reactivateHistoryInput struct {
    ChainID core.TaskChainID
    TraceID core.DayTraceID
    Today core.LocalDate
    Now core.Instant
    EventIDs []string // P结果恰零；U结果恰一；不接受caller选择capture status
}
func auditAbandonHistoryVNext(prior validatedHistorySnapshot,
    before, after core.TaskGraph, in abandonHistoryInput) (auditedHistoryCommand, error)
func auditReactivateHistoryVNext(prior validatedHistorySnapshot,
    before, after core.TaskGraph, in reactivateHistoryInput) (auditedHistoryCommand, error)
```

Reactivate 的 EventIDs只携带新事件身份，不选择状态。先 metadata限制至多一项，重放后内部按实际P/U状态决定恰需0/1；不匹配返回 `historyCommandCount`。P接受nil或空slice，U恰用EventIDs[0]；结果不保留该slice，也不返回未经aggregate audit的core trace ID。

新增闭合 kind `historyCommandAbandon`、`historyCommandReactivate`，`valid()` 用显式穷举所有五种 kind，避免放宽数值区间接受未知值。保留既有三种结果行为不变。proof 绑定已有 kind/today/now/chain/trace/prior/after/changed，并新增仅本批有意义的标量 `effectiveTrace core.DayTraceID`、`eventID string`。Abandon 保留请求 TraceID 与内部解析的实际 TraceID 两者；Reactivate effectiveTrace 必须等于输入 TraceID；P时eventID为空，U时eventID为EventIDs[0]的调用时字符串值。Abandon 不消费 Today，proof 的 today 保持零值，不能制造日期证明。eventID 保存调用时的精确字符串身份；新 event 的 UUID uppercase规范化仍由 builder 执行，测试同时断言两者约定，不能混淆新 ID 规范化与旧分类字节保留。

所有错误返回零 `auditedHistoryCommand`；其 changed/snapshotCopy 均报 proof 错。结果不留 caller slice/map/pointer。普通 Go string 值为不可变标量；不承诺 unsafe 或调用期间并发写入安全。每次 snapshotCopy 返回独立 owned copy。

### ID／分配账本

| 命令 | 新事件 | 新 Day／Trace／Definition／child | ID maps |
|---|---|---|---|
| Abandon 成功 | 恰一 EventID；内部 A@Now，绑定实际 target | 全为零 | 无 |
| open current/future Reactivate P | 零；旧 A 完整保留 | 全为零；返回原 trace | 无 |
| past/locked Reactivate U | 恰一新 U EventID；旧 A 完整保留 | 全为零；返回原 trace | 无 |

不存在 random、clock读取、UUID allocator 或新 Day/Trace/subtask mapping 参数。fixture 的 Create/Schedule/Continue 可能需要这些身份与映射，但它们属于前态构造，不属于本次命令输入。若 after 新增 Day/Trace/child、复制内容、改 lineage，精确 replay 必须拒绝。Abandon标量 EventID固定一条；Reactivate先拒绝EventIDs长度>1，再按replay状态精确检查0/1。空或长度错误ID失败，caller不可提供第二条。成功的两个命令都改变状态，因此不是no-op；零capture的P reactivation也changed=true。

## 4. 入口顺序与资源合同

两入口都必须具有可由测试 AST 审查的直线前置段，每步失败立即返回：
1. `prior.valid()`（只读 seal，不遍历 caller graph）。
2. `historyCommandGraphBudget(before, historyCommandBefore)`。
3. `historyCommandGraphBudget(after, historyCommandAfter)`。
4. 输入ID metadata gate：Reactivate先检查 `len(EventIDs)<=1`，超限在访问元素前拒绝，然后检查ChainID／TraceID及可选唯一EventID均len==36；Abandon检查ChainID／TraceID／EventID均len==36。只读取最多三个header，O(1)，无slice/map分配或UUID扫描。
5. 才能 bind、copy、ToDomain／FromDomain、Validate、kernel clone、索引／UUID内容扫描等。

复用既有 exact-model预算：五数组 base nodes 5，Day17／Chain13／Definition19／Trace33／Subtask25，加全部 optional、notes11、planned17等现有成本；结构遍历结束后才累加各 string raw len，每次逻辑出现都计费。before/after各自独立 N/B 预算，不叠加为一份，不使用最坏转义倍数误杀合法输入，不按共享地址去重。沿用 model manifest／每字段optional节点对账，不复制维护另一套更弱预算。

输入总原始ID字节上限均为108（Reactivate P为72）；36长度只保证工作量，不证明 UUID合法、唯一、归属。后续 UUID扫描采用现有检查，旧事件冲突（含大小写）、revision溢出、classification node/byte/depth预算继续由 builder把关。最终完整 snapshot的 N/B/depth由 inspector检验；前置 budget是下界，不是 envelope可接受性或精确RSS保证。

当前 AST gate只覆盖两个老入口，必须明确扩充到新文件／新入口，并审核新 ID预算helper只含 metadata允许调用。结合同时 budget-invalid/domain-invalid输入测试证明 before/after/IDs错误优先。不能仅凭最终error类型声称形式化控制流证明。

## 5. 精确 replay、内部 capture 与固定围栏

### 5.1 共用管道

所有 gates 后，调用现有 `historyCommandBind(prior,before)`：从 owned prior 解码原 state与extra，精确比较 before组合及prior自身round-trip；不采纳 caller graph作真实重放底稿、不用 emptySupplement洗白。校验输入 UUID、finite Now（Reactivate另校验Today），保留 owner先于redirection的语义。

只在从 prior取得的owned graph上建立 kernel，调用对应 core typed API，取得 replay；`historyCommandMatch(replay,after,extra)`比较完整 task graph wire表达。after不得携带自选classification。再调用 `historyCommandFinish`，完整 append→compose with original supplement→inspect→prefix→supplement fence→exact count。最后返回闭合结果；changed根据完整prior/final精确相等计算，不根据capture数量。

### 5.2 Abandon

- 先按请求TraceID找到trace并核对ChainID，再找该chain。错误owner不能利用同链另一pending被重定向。
- 内部在owned prior中独立解析实际target：若该chain存在pending就使用它，否则使用请求trace。已有core validator保证每chain至多一个pending；不按after差异或caller hint选target。使用与core一致的active规则，不把unfinished当active。
- 精确调用 `k.AbandonChain(core.AbandonChainInput{ChainID,TraceID,Now})`。实际target必须原P/U，chain active；不得改变当前core状态／clock合法域。
- 成功replay实际target.status=A，contentUpdatedAt=Now，settledAt=Now，pin=nil；owner chain.state=abandoned，updatedAt=Now。其余所有字段原样，包括历史请求trace（若不是实际target）、children、manual progress、notes、definitions、days。
- 内部唯一intent `{EventID:in.EventID, TraceID:effectiveTrace, Status:A, At:in.Now}`；从before解析和core replay双重核对，不读取after status决定capture。向旧stream追加而非新建重复pair；revision+1；既有 event原字节／次序／旧key表示不变，latest只按builder规则替换目标。
- 固定补强围栏：days/definitions/subtasks全等；所有非owner chains、非effective traces全等；目标的非许可字段由exact replay冻结。不是对某类字段一概豁免。

### 5.3 Reactivate内部派生0／1 capture

精确调用 `k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID,TraceID,Today,Now})`，检查返回ID等于输入TraceID。不得进行Abandon那样的active重定向。

| source day与Today | core精确状态／settledAt | 内部capture |
|---|---|---|
| open，date==Today | P；settledAt=nil | 零；旧A/latest/revision原样 |
| open，date>Today | P；settledAt=nil | 零；旧A/latest/revision原样 |
| open，date<Today | U；保留旧A settledAt | 恰一U@Now，追加旧stream |
| locked（任何date比较） | U；保留旧A settledAt | 恰一U@Now，lock优先 |

全部分支写source.contentUpdatedAt=Now、pin=nil，chain active／updatedAt=Now；不复制/结算子任务、notes、new Day或trace。有效A必已有settledAt；core的nil补值分支不能用来修复非法prior。

独立按owned prior的锁与日期计算预期P/U，再检查core replay结果一致。P要求len(EventIDs)==0；U要求==1并构造唯一 `{EventID:in.EventIDs[0], TraceID:in.TraceID, Status:U, At:in.Now}`。不能从caller after派生capture。U的capturedAt/contentUpdatedAt是新Now，SettledAt保留旧A的exact bits，二者不能混为同一时间；旧U/A等events均保留，latest更新为新U。

days/definitions/subtasks冻结，只有请求trace及owner chain可有精确replay差异。P追加零intent后整个classifications必须exact相等；U按builder追加且通过完整inspector/prefix，revision恰+1，无关stream/latest及旧目标prefix保持。既有 `[U,A]→[U,A,U]`、open `[A,U]`后silent lock都属于必要组合测试。

负例必须证明zero-capture U失败，以及P多传一个EventID失败；这不是将零-capture冲突隐藏掉，而是按完整来源决定的不同分支。任一步失败返回零结果，输入/旧proof不变。

## 6. 时钟与表示

服从core局部trace/chain nondecreasing规则及fixed全事件顺序，不额外加入全snapshot严格frontier。Now finite；数值相等（含signed zero）与MaxFloat的pure-helper接受性由core+profile判定；赋值和比较证据用exact bits，禁止取整／nextUp／墙钟或把Now换成旧settledAt。其他chain较新clock本身不构成global拒绝理由；但新A早于已有全局event必须被完整inspector拒绝。locked日来源若core成功、profile不满足仍失败，不修时间。

保留原supplement全部表示，不能统一uppercase旧events/key；prior任务rawValue若bridge round-trip改变则binding失败，沿用已接受政策。新event ID builder规范化不构成对旧表示的改写许可。

## 7. 未来取消与消费者边界

`future P→A→Reactivate P`保存A。随后旧core Return可以造X，但fixed profile拒绝X+A；补核指出Swift完整snapshot同样禁止X历史分类。保持此负例，不把它写成应成功的正例，不删A，不隐藏X事件，不声称Swift Store已成功或当前v2回归。需要未来产品准入与完整事务差分另议。

Service全消费者切换须另批：现旧reader甚至拒绝无关chain A；本批不解此问题。纯result也不能直接供未来commit信任：还需要handle当前prior、request/head、统一mutation frontier、完整disk prefix、canonical round-trip与故障/重开/并发发布协议。

## 8. 风险、灰度及回滚

P级数据证明设计；风险包括误把请求trace当target、重复capture／漏U、使用after生成意图、旧supplement丢失、scalar身份未绑定、预算后置与alias污染。采用下附TEST-ORACLE全部门禁，不以文档代替运行证据。

灰度仅隔离pure helper，无运行用户／storage写入／UI。回滚为撤下本批未接线helper和测试，保留既有profile/历史/会话；不迁移、不删资料、不清缓存回避失败。先独立设计复核，确认来源派生0／1合同，然后才最小实现；后续验证命令及证据范围须由实施任务记录，本轮没有运行结果。
