> 冻结规格 R3 于 2026-11-06 获独立审查 accepted。下文的“未实现／待复核”是规格冻结时的历史状态；当前阶段 1 实现与实际验证见本目录 DESIGN.md、VERIFY.md。未来 writer/store/UI 路线仍未接通。原始规格 SHA-256：e45ec63d1766dd6c74f1826a500333778cc8022797b7aa271a8b2618377839a3。

# 普通任务历史审计：未接线纯模块设计 R3

task-id: connect-history-chain-service-audit

日期：2026-11-06。级别：P。状态：根据独立审查修订的设计候选，等待复核；本轮只改外置文档，没有实现或测试运行。

## 1. 本次收口与当前交付边界

R3 取代 R2 作为下一阶段设计候选，仅修正 §5.4/§5.4.1 的终态 settled token 表达，不扩大 scope。R2 原文与冻结哈希保持不变；原 R1 也原位保留并有逐字归档，其误宽承诺和遗漏不再作为实现依据。对应原三项审查报告：`/workspace/shared/noonmark-history-chain-audit-independent-20261106/REVIEW.md`。

**下一阶段仅设计为三个未接线、package 内部的纯职责：**

1. 完整 snapshot inspector：固定 forward-clock ordinary profile，检查结构、空 catalog scope、全局 events 和逐流必要一致性。
2. append-only builder：只处理已验证旧分类事实和显式 capture intents；event ID 由调用者预先提供，builder 只分配连续 classification revision。
3. successor checker：检查两个分别验证过的 snapshot 之间旧 event 前缀和日锁事实的保留。

下一阶段不替换 `ValidateEmptyHistory`、`CaptureEmptyHistory`、`encode/decodeSnapshot`、`Open`、`Service` 或 UI 的任何现有调用，不接磁盘写入、importer 或新 runtime reader。当前 narrow reader 的所有拒绝门禁继续生效。本轮更不开始上述模块实施。

明确固定以下技术决定，不再作为需要用户重做的选择：

- 候选 profile 仅一套内部固定语义，不提供 `allowAbandoned`、`ignoreClock`、`allowLockedChanges` 等任意豁免 flags。
- forward-clock 是本 profile 的边界，不承诺接受所有任意 purecore Now 或 Swift 来源输出。
- open-U 只保留为**source-supported、forward-clock 的合成只读扩展**；它不是正常 Service 可达性保证，也不授权写入或导入。
- 逐流状态机只证明必要一致性，不证明完整 aggregate 可由支持的 Service 序列生成。
- 未来 writer 使用全新 development v3 会话；v2 不迁移、不覆盖、不删除、不原地继续写。不改 snapshot wire v7 来冒充 runtime 兼容。
- 不新增完整 command log；不将 hash、UUID、可能的历史 witness 当成真实动作或用户授权证明。

## 2. 冻结基线与证据等级

正式只读仓库：`/workspace/scratch/586e77bd59a6/noonmark-mygo-work/noonmark-go`。

本轮重新读取确认 HEAD 为 `b8857778b990fd30dae35c24b03fb49c08c0cf12`，`git status --short` 无输出。purecore 与相关批次已本地提交，不再沿用 R1 的“尚未 commit”现状叙述。基线已有 check/cross/build/CLI 验收记录；本设计编写阶段没有重跑，不把它算作本设计的实现验证。Git 写操作仍暂停。

只读上游：sibling `noonmark-reference`，HEAD `43e6eefaae3a04be89cab3e234d470111afd80ef`。继续遵守已读的上游 AGENTS.md 与 Go `ports/mygo/AGENTS.md`。本轮重新核对 core 的 abandon/reactivate、complete/undo、settle 与 LocalDate 边界源码。

R1 冻结 SHA-256：`79547c4196fcf2274409af305c9bf92c7f6549d67e1ff9660fcdd1e2d5ec3a27`。

独立审查报告 SHA-256：`4c346881d6389f2ca0159e8ec78581e3776ded61ae22dda68e5fcdefc9729923`。

本报告仍为源码级设计证据。没有运行 Go/Swift、构建、UI、production、iCloud、Keychain、真实 AI、GitHub 或远端动作；没有扫描 home、删除缓存、复制 toolchain。独立审查的 Python 抽象枚举是审查方的单 trace 研究，不是 Go/Swift 或 Service 验证。

### 2.1 精确来源

Swift 相对路径从 `Sources/NoonmarkCore` 起算，测试另标；Go 相对路径从 `ports/mygo` 起算。

| 来源 | 本设计使用的合同 |
| --- | --- |
| Swift `NoonmarkEngine.swift:2513–2572,3814–3819` | abandon 请求与实际 pending 目标可不同；reactivate 原位，锁日优先；P 不 capture，U 保留 settledAt 并 capture |
| Swift `NoonmarkEngine.swift:1996–2016`；Go `domain-core/core/kernel.go:240–312` | complete/undo 不 capture；最后一次 complete 把实际 Now 写入 completedAt |
| Swift `TaskClassification.swift:1417–1474,4022–4059` | event 字段、append stream、替换 latest、revision+1 |
| Swift `TaskClassificationStateValidation.swift:25–50,405–452` | 原结构 validator 的计数、stream/latest、引用、ID 与 revision 范围规则 |
| Swift `NoonmarkSnapshot.swift:82–83,189–194,440–527,548–617` | 完整校验；X 不可带历史；对象内时钟与引用 |
| Swift `NoonmarkEngine.swift:2019–2065,2152–2260,3017–3071` | 当前退池/延期 capture；future cancel/continue 不 capture；settle 仅捕获本轮 P→U 与 C |
| Swift `MutationClock.swift:5–40,43–107,168–173` | runtime 时钟跟随全部持久化 facts，含全部 classification events |
| Swift `TrajectoryTopologyValidator.swift:30–85,228–246` | 对象内 clock；A 不能作正常 continuation source |
| Swift `Tests/NoonmarkCoreTests/TaskClassificationModuleTests.swift:1438–1472,1882–1920` | completion 到结算前不 capture；重复 abandon 保留 `[A,A]` |
| Go `domain-core/core/history_chain.go:26–149` | owner fence、目标重定向、局部 clock、原位 reactivation |
| Go `domain-core/core/kernel.go:319–383`、`return.go:21–186`、`carryover.go:28–220` | settlement、退池、延期/延续的 core 组合语义 |
| Go `domain-core/core/time.go:15–113`、`validation.go:68–84,103–125,196–197,232–351` | IEEE bits、合法年份 1…9999、内容/终态 clocks、拓扑 |
| Go `internal/taskapp/history.go:21–136,143–341` | 活跃 narrow reader；generic 历史冻结；尚只支持首次 capture 的逻辑 |
| Go `internal/taskapp/{return,carryover}.go` | 已有专用整图 replay；原追加 map pair 不能直接支持多事件 |
| Go `internal/taskapp/service.go:49–132,181–236` | Open/AdvanceTo 的 reconcile/Today 发布；统一 mutation clock |
| Go `internal/taskapp/store.go:39–44,91–151,237–353,387–451` | 记录格式、所有历史 revision 读取、提交/未知结果、scope/prefix |
| Go `domain-codec/v7/references.go:103–157`、`schema.go:56–70` | schema 与基本 event 引用；不能替代 runtime audit |

## 3. 三层权威与保证上限

| 层 | 证明内容 | 不证明内容 |
| --- | --- | --- |
| 上游结构 validator | 单 classification state / 完整 Snapshot 的索引、计数、引用、X 无历史等约束 | 不证明操作资格、真实动作顺序、来源真实性或授权 |
| 固定 profile inspector | 本 profile 的 scope、全局 event 规则、每条 trace 与锁日/存储字段的必要一致性 | 不证明所有 trace 共用的真实事务批次，不证明完整 Service 可达性，不批准写入 |
| 后续专用命令 replay | 已知 prior/before/after 和明确输入的整图精确匹配 | 不恢复未记录的过去命令，不替代独立源语义 oracle 或磁盘确认 |

上游 stream validator 没有要求 latest.status 始终等于 current.status，也未要求 capturedAt=settledAt=L。其 per-stream revision 递增、总计数和范围，不等于跨流 revision 已严格无重号覆盖 1…N。后者与 capture 非倒退是当前 Go 受限 runtime 的更强检查，不能写成上游已保证。

### 3.1 为什么“全部检查通过”仍不是 aggregate 可达性证明

例：x/y 两条不同 chain 的 trace 在同一天，z 在另一日。全局事件为 `Ux@10 rev1, Az@10 rev2, Uy@10 rev3`，x/y 同日 L=10，其他字段自洽。

每流状态机、全局连续 revision、非倒退 capture 都可能通过。但一次 settlement 必须在同一命令内收集 x/y，另一 abandon 不能插在两次 captures 之间；严格 Service clock 也不会让两个独立成功 mutation 都用 10。

inspector 可以把它判为“必要条件通过”，**不得**称为“已找到一条真实或完整可能 Service 序列”。阶段 1 没有 importer/reader 接线，不能将通过值直接持久化。未来写入仍须 typed replay 与完整事务。相邻 snapshot 的 prefix 检查也不能补上未记录的命令批次。

## 4. 固定 forward-clock ordinary profile

内部名称建议 `forwardClockOrdinaryHistoryVNext`。这是单一固定语义标签，不是可配置框架，不是新的持久化 schema。以后 v3 session 映射到该 profile。

### 4.1 纳入范围

- 普通任务、空 catalog、无 taskCycleSeries/ideas；neutral preferences、无 CycleMembership。
- 状态集合 P/C/U/A/R/D/X；changed 及其它未接通事实拒绝。
- schedule/create、complete/undo、settle、abandon/reactivate、current return/future cancel、defer/continue，以及既有 title/manual/subtask 命令的分类历史投影。
- 声明的 mutation 时间向前；不同 Service mutation 的 Now 严格前进，同一次 settlement 多 event 同时刻。inspector 为必要检查接受相等事件时间，**不能仅凭相等认定同一事务**。
- 一个明确的合成只读扩展：open A→past reactivate U，在保持时钟向前时以后可锁日或自然日倒退。该扩展用于 source-supported reader/helper 测试，不承诺正常 Service 能生成或导入它。

### 4.2 必须拒绝而不得归罪源模型的反例

从 open P、day/trace/chain clocks=0 起：

1. abandon Now=10：A@10，trace.contentUpdatedAt=settledAt=10，chain.updatedAt=10，day.updatedAt 仍为 0。
2. settle Today=d+1，Now=5：已有 A 不进入 pending 循环，只把 day.updatedAt=lockedAt=5，无新 event。

Go `history_chain.go:43–51,143–146`、`kernel.go:333–367` 与 Swift `Engine:3023–3068` 的局部检查允许此结构；对象内 clock validator 没有对 A 与 L 增加这种跨对象顺序。

因此 `[A@10] + L=5 + current A@10` **不是已证明源模型无效**，也不是正常 Service 漏掉的合法 forward-clock 路径。它是本 profile 的 `unsupported history`。本设计撤回接收全部 purecore/Swift 输出的隐含承诺；不修改 core，不放宽状态机去接它。

正常 profile 正例包括：

- `[U@5,A@10] + L=5 + current A@10`：先锁 P，再 abandon U。
- `[A@10] + L=20 + current A@10`：先 abandon，再更晚锁日，已有 A 不 capture。

event.clock 与 L 的大小可用于排列动作，**依赖本 forward-clock profile**，不是 Swift snapshot 公理。R/D.capture≤L、锁日 C 的 completed/content≤L、day lock 不早于该日 trace 创建，同样是本 profile 必要限制。

### 4.3 open-U 与实际 Service 的分界

从全新 Service 经 Open/AdvanceTo，过去日会先 settle，成功后才使用新 Today。因此“open A→past reactivate U”不作为新 Service reachable 的证明。

阶段 1 可以构造合法 source-supported synthetic candidate：A@10→open U@20，settledAt 保留 10；以后 Lock@30 保留 U@20，不追加第二条 U。这证明纯 inspector/helper 对该明示扩展的兼容性，不能写成 Service 已产生/写入/回读此组合。

现有 `history.go:65–68` 拒绝 unlocked U；`297–305` 的 after U/C capture 筛选只在未来扩容后才会有重复 U 风险。没有当前用户资料已发生该 bug 的证据。

## 5. 事件投影与逐流状态机

### 5.1 事件动作表

P=pending，C=completed，U=unfinished，A=abandoned，R=returnedToPool，D=deferred，X=cancelledDraft。方括号表示追加本 trace event；ε 表示没有 classification capture，不表示没有任务修改。

| 动作 | 投影 | 限定 |
| --- | --- | --- |
| complete/undo | P↔C，ε | open、Today=date；最后一次 complete 必须绑定实际 completedAt |
| settle | P→U[U]；C→C[C] | 本轮新锁、存在合法 Today>date；已有 U/A/R/D 不 capture |
| abandon 实际 target | P/U→A[A] | chain active；有同链 pending 时 target 重定向；请求 source 不一定改变 |
| reactive P | A→P，ε | open、存在合法 Today≤date；旧 A/latest 保留，settledAt 清除 |
| reactive U | A→U[U] | locked，或 open 且存在合法 Today>date；保留原 A 的 settledAt |
| current return | P→R[R] | open、Today=date；旧 stream 追加，不覆盖 |
| future return | P→X，ε | open，actual draftCancelledOn<date；旧历史非空则完整 aggregate 拒绝 |
| defer | P→D[D]，另建 P | current source、有效未来 target、frontier 等完整资格由 replay/拓扑负责 |
| continue | U→U，ε，另建 P | 源整图与 history 保留；完整资格由后续 replay 负责 |
| title/manual/subtask | ε | 现有精确命令资格不变，不删除旧 stream |

新目标 trace 不复制源 classification stream。以历史 C/R/D/X/U 请求 abandon，在同链有 pending 时只给 pending 实际 target 追加 A；不能按请求状态或 ID capture。

### 5.2 必要邻接规则

- 首事件只可 A/U/C/R/D。
- A 后可 A/U/C/R/D；中间未 capture 动作不能凭空认定实际发生。
- U 后只可 A；UU 拒绝。
- C/R/D event 为该 trace 的事件终点。
- P/X/changed 不作本范围的 capture event。

这些只是必要规则。`UAA/UAC/UAD/UAR` 可通过邻接，但首 U 由 lock 产生后无法回 P，必须由锁日状态机拒绝。不能把 inspector 简化成字符串邻接。

### 5.3 合法 Today 存在条件

LocalDate 域为 Gregorian `0001-01-01…9999-12-31`，来源 `time.go:107–113`。未存 Today 只意味着不能恢复真实取值，不意味着可以使用不存在的日期。

- open A→U 与 Lock(date) 必须满足“存在合法 Today>date”，等价于 date<9999-12-31。
- 任何候选 snapshot 的锁日在 9999-12-31 上均不属于本 profile；不得用虚构 10000-01-01 解释。
- open A→P 需要存在合法 Today≤date，对任何合法 date 都可选 Today=date。
- complete/current return 需要合法 Today=date，合法 trace date 已足够提供存在性。
- future X 使用实际存储的 draftCancelledOn，并验证其合法且严格早于 trace.date；不另猜 Today。0001-01-01 的 future X 因没有更早合法日期而拒绝。
- defer 所需 target>source.date 等完整结构检查保持；不在最大日期虚构未来 target。

本节仅检查存在性的必要条件，不保存/恢复过去 Today，也不枚举公历日期。

### 5.4 有界状态扫描

状态为 P/C/U/A/R/D/X 加 open/locked。初始为 open P；每个日锁至多一次，使用存储 L。若该日无 L，不插入 Lock。

1. open P↔open C 可作为无 capture 闭包；open A→open P 可作为无 capture reactivation；无历史 open P→open X 可作为 future cancellation。均受第 5.3 节日期条件与可用时间区间约束。
2. A→U 消费 U event，保留前 A 的 settledAt token；如果 open，另检查存在合法 Today>date。
3. P/U→A 消费 A event，settledAt token 改为该 A.captureAt。
4. open P→R/D 消费对应 event；实际 final settledAt/contentUpdatedAt 必须与 terminal captureAt bit-exact 相等，直接按 §6.2 核验，不把该时刻装入后续 reactivation 的 settled 来源 token。R/D/X 均为终态；X 的取消时刻绑定实际 final settledAt/contentUpdatedAt，并核验实际 cancellation witness。三者的 token 槽均标为 not-applicable，不代表存储 settledAt=nil；其后无同 trace 状态动作，仅可能有日锁。
5. Lock(L) 必须有合法 Today>date：P 必须在 L 消费 U，C 必须在 L 消费 C；U/A/R/D/X 只锁日，不新增 event。
6. locked A→P 不存在；locked A→U 消费 U；locked U→A 消费 A；locked C/R/D/X 不再变化。
7. final state 必须等于 current trace；current P 必须 open；current X 无历史。最后 event 和 final 字段还须通过第 6 节。
8. event.clock<L、>L 在本 profile 内限制 Lock 相对顺序；==L 时保留有限个与来源相容的顺序，不把 bits/UUID 排序误当未存储的动作次序。

ε 动作只能在不早于相关已存 clock、不晚于下一可见事实/最终内容时钟的有限时刻发生。这只是可能的 witness，不证明用户做过它。

**final C 的最后一次 complete witness 必须 bit-exact 等于 trace.completedAt。**不能在区间内任选另一个时间后保留实际字段。合并状态必须保留这个固定时间约束：可把 final completion 设为受 actual completedAt 约束的 ε 边，或先做等价的显式末 A 下界检查并保证路径的最终 complete 正是这个值。不能仅以“state=C、lockConsumed、settledAt token”合并丢掉约束。

实现必须采用封闭有限状态，而不是枚举任意多条 ε witness。下面给出可供实施/独立测试的明确上界；若实施选不同等价算法，也须给出不更弱的有限状态归并证明。

#### 5.4.1 状态归并与扫描工作量

在一个固定 event index，最近已消费 A 的 bits 与存储 L.bits 都是已知常量。候选键可取：

- 当前状态：7 种。
- lock-consumed：0/1。
- settled 来源 token 按当前状态解释：P/C/U/A 使用 nil / 最近 A.bits / L.bits，至多3种；token 含来源标签和原始 bits，即使数值相等也不合并不同来源或 +0/-0。A 自身的 token 在消费 A 时更新为该固定 index 的最近 A。R/D/X 只使用一个 not-applicable 槽，其余两个槽不用；这不是第4种可变 token，也不表示实际 settledAt=nil。它们的终态字段由 §6.2 直接核验。
- final-completion-anchored：0/1，2 种；actual completedAt 是输入中的固定值，不是可自由枚举的时刻。anchored C 后不再允许 undo 或其它会使该完成失效的动作；有这种冗余循环的可能路径，可选择其最后一次完成作为 anchor。

R/D/X 没有后续 reactivation 或其它 trace 状态动作；仅可能追加一次日锁，所需时序下界仍由其已消费的 terminal capture 或实际取消时刻保存。实际 settled 字段的精确核验独立保留，因此终态不携带 settled 来源不会丢失后续可行路径。

因此每种当前状态最多占3个 token 槽，可为每条流的扫描位置预分配最多 Q=7×2×3×2=84 个槽，非法组合直接不用；不为“历史路径”分配对象。每槽保存最早可行的**数值**时刻及所需上界检查，原始 Instant bits 的复制来源另由 token/actual completedAt 保留。归并不得仅用 float 相等比较 settled token。

在完全相同键下，较早的可行下界支配较晚下界：后续边只要求不早于该下界、通过固定合法日期 guard、满足固定事件/lock/final completion 时刻；exact bit 关系已放入 token 和 anchor 约束。证明支配关系前不得删除候选。actual completedAt 的固定时间边既检查 lower bound≤completedAt，又把后续下界设为该实际值；不可用任意邻近 witness 替代。

P↔C 的无事件中间循环没有新历史事实，且只会保持或抬高时间下界。回到同一完整候选键的循环不可能降低下界，因此可移除而不丢存在性路径。ε 闭包用有限图的固定点计算：最多 Q 轮、每轮遍历固定数量的状态边即可覆盖简单路径；不枚举循环次数。相同数值的多个顺序/bit token 分支在各自槽内保留。若发现所需约束不能由此有限键/下界保存，应停在设计层增加有来源依据的**封闭状态**并重新证明上界，不能靠超时、重试、路径数量截断或宽松接受兜底。

新增扫描部分的保守时间上界为 O(E log E + (E+T)×Q²)，其中 E 是全部 events、T 是 traces，Q 固定为不超过84；全局排序用于 revision/clock，对每流只扫描一次并计算常数状态闭包。新增存储为 O(E+T+Q)，不保存整段 witness 路径。现有 v7/graph 验证的工作量另按既有受限输入合同测量，不把它冒称为此新扫描的线性复杂度。

输入先经过第6.1节 bounded roundtrip。当前 canonical 默认上限为16MiB、深度128、节点1,000,000（`domain-codec/canonical/canonical.go:43–61`）；E/T计数与所需切片容量必须checked，不能先按未验证的revision申请内存。最大允许节点/事件输入的测试同时检查状态槽不超过Q、ε轮次不超过Q；不以wall-time超时作为正确性算法。builder也须一次复制旧分类、按索引追加，不能每加一个intent就全量重新复制/验证旧历史造成隐蔽二次增长。

状态机通过只表示逐流必要一致性通过，不表示已重放 source/target routing、child 编辑、pool 资格或跨流原子批次。

## 6. Inspector 的完整检查合同

### 6.1 结构、scope、全局 events

输入为 complete canonical snapshot。采用现有 `v7.Inspect` 的 bounded roundtrip 取得独立对象：构造formatVersion=7的完整envelope，先用受限 `canonical.Marshal` 检测caller-created AST的环/深度/节点/字节边界，再用 `v7.New(v7.Options{}).Inspect` 检查canonical/schema/core并取得其deep-cloned Snapshot。对应现有来源为`domain-codec/v7/codec.go:34–82,98–108`与`internal/taskapp/history.go:342–348`。不调用已接旧audit的Encode/Decode来产生循环依赖，也不把Inspect本身视为分类审计完成。

随后显式检查完整supplement/runtime scope、TaskGraph及本profile历史；不能只检查event list，也不能用codec空分类shortcut跳过必要历史。所有后续索引基于已独立拥有的bounded对象建立，不在验证前任意clone未受限AST。

- catalog categories/labels/current/relationHistory/management/receipts/changeRecords 全空；ideas、series 空；preferences 精确 neutral；CycleMembership nil。
- 不修改原数组或“修复”历史；任何未知字段/非法 schema/enum/ID/日期/clock/拓扑继续拒绝。
- latest keys==stream keys；每流非空；key 与 event.traceID 一致且引用存在 trace；latest 与尾 event canonical 完整相等。
- event category 不存在，labels 空；UUID 合法并在所有 streams 全局唯一，大小写规范化后比较。latest 是索引，不重复算一个 event。
- 事件数 N 使用 checked arithmetic；classification.revision=N；event revision 恰好 1…N，各一次；原 stream 顺序严格递增，不排序后修复。
- 按全局 revision，capturedAt 有限且非倒退。使用 Instant.Seconds/Before；uint64 bits 不代表时间大小。允许相等并不证明真实同事务。
- capture 不早于 trace.CreatedAt；有 L 的日锁不早于其该日 trace 的创建，且 date<最大合法日期。此为固定 profile 约束，不是原 validator 的全面来源公理。
- 已知同一个 Now 赋值的字段 bit-exact 相等；仅时序/max 使用数值比较，保留 +0/-0。
- A/U/R/D 和 locked C 必须有历史；unlocked P/C 可无历史；X 必须无历史；locked P 拒绝。

不在阶段 1 添入“chain abandoned iff 某条 A”等整个 aggregate 可达性公理。现有 graph 自身约束照常检查；超出 graph 的 chain 改变资格交给后续精确 replay。

### 6.2 current/latest 与时间约束

下表必须和完整 stream/锁日扫描共同执行，不是仅凭末项的豁免。

| 当前状态 | 必要关系 |
| --- | --- |
| open P | 空 stream 或尾 A；settledAt=nil；尾 A.capture≤contentUpdatedAt；存在与完整 stream 相容的 open reactive P witness |
| open C | 空 stream 或尾 A；core clock 有效；最后 complete witness=actual completedAt；有 A 时末 A.capture≤completedAt≤contentUpdatedAt |
| locked C | 尾 C，C.capture bit-exact=L；final complete witness=actual completedAt；有 A 时末 A.capture≤completedAt；created/completed/content≤L |
| A | 尾 A；A.capture=settledAt=contentUpdatedAt；L 可早于 A，但必须有状态机相容前缀，例如 `[U@5,A@10]`，不是任意单 A/L 豁免 |
| U，settlement 候选来源 | 尾 U；U.capture=settledAt=contentUpdatedAt=L；状态机必须真实消费 Lock 的 P→U 边 |
| U，reactivation 候选来源 | 紧前 event A；U.capture=contentUpdatedAt；settledAt bit-exact=前 A.capture；A.capture≤U.capture；open 时要求合法 Today>date；L 可在 U 前或后 |
| R/D | 尾对应 event；capture=settledAt=contentUpdatedAt；若日已锁，capture≤L；旧 A 保留；D 的 target 继续由拓扑验证 |
| X | 无 latest/stream；settledAt=contentUpdatedAt；actual cancellation witness/date 通过现有 future cancellation 规则 |

U 的来源在相等时钟下可重叠，检查明确的或条件，不认定唯一真实来源。不要顺手增加 `C.contentUpdatedAt==completedAt` 作为所有旧结构的通用公理；写时 replay 与历史结构检查是不同权威。

### 6.3 completedAt 阻断反例与正例

必须加入直接规格测试：

- **拒绝**：created=0；stream `[A@10 rev1,C@20 rev2]`，L=20，current C，completedAt=5，contentUpdatedAt=15，settledAt=nil。对象内 clocks 和上界都合法，但末 A@10 后的最终完成不可能留 completedAt=5。不可任选 complete witness=12 来蒙混。
- **通过必要检查**：A@10→reactive P@11→complete@15→Lock@20；stream `[A@10,C@20]`，completedAt=15，contentUpdatedAt=15，L=20。这里是明确来源构造的正例；inspector 不会从 snapshot 恢复实际 reactive P@11。
- open C 的同类正反例也保留。相等时间时采用 `末 A.capture≤completedAt`，不把 core 允许相等误写为全源严格小于；Service 不同 mutation 严格前进由未来 writer 保证。

### 6.4 不能从一个 snapshot 证明的历史

可以拒绝缺必要 event、错引用、重复/缺口 revision、倒退 capture、stale latest、UU、terminal 后 event、locked P、错误 retained settledAt、X 带历史、非空 scope。

不能证明旧 A1 曾存在后被删除重编号、真实 Today/Now/requested source、capture 间真实 complete/undo、改过再改回的内容、跨 trace 原子批次或用户授权。整体伪造但自洽的 snapshot 可与合法历史不可区分；不能补造历史来“修复”。UUID/hash chain 也不是签名。

## 7. 三个独立纯职责的 API 建议

所有名称为候选内部名称，具体 Go 命名由实施评审决定。不导出新 Service API，不赋予外部 caller 操作权限。

### 7.1 完整 inspector

概念接口：`inspectForwardClockOrdinaryHistoryVNext(snapshot) (validatedHistorySnapshot, error)`。

- 受限深复制并检验完整 snapshot；返回私有构造、带固定 profile 标识的只读 validated snapshot/index。
- 返回值不得泄漏底层 map/slice/pointer；后续查询也返回副本或不可变值。未经构造器不能伪造 validated 类型。
- 分类 index 包含全局 UUID、revision/count、各流和尾部、必要 day/trace 引用；不是操作 capability。
- 失败返回 typed reason：例如 malformed structure、unsupported scope/profile chronology、missing capture、invalid event order、invalid completedAt witness、no legal Today、invalid prefix。具体 reason 不输出任务正文。
- 不分配 clock/UUID/revision，不改原输入，不做 repair，不访问磁盘。

### 7.2 确定性 append-only builder

概念接口：`appendEmptyHistoryEvents(validatedPrior, intents) (classificationCandidate, error)`。

intent 显式含 `EventID, TraceID, Status, At`；category/labels 固定为空。event ID 在未来上层事务准备阶段生成，**helper 不随机生成 UUID**。固定输入得到固定输出。

- prior 必须属于固定 profile；输入 intents 是要做的 capture 数据，不是动作授权。
- 检查 ID/允许的 capture status/finite At、全局 ID 唯一、intents 内重复 ID、revision checked increment、容量；保留旧流顺序和所有旧 event 内容。
- existing stream 尾部追加；latest 对 existing key 原位替换；只有新 key 才新建 map pair。零 intents 返回相同分类事实，不消费 revision。
- 不从 after.status 猜目标，不判断 abandon routing、资格、Today 或 lock，不检查尚未收到的 after graph；trace 引用及 final 状态必须在最终组合 snapshot 的 inspector 中确认。builder 自身结果只是 classificationCandidate，不是 validated aggregate。
- 不排序/丢弃/覆盖旧 events，不提供字段豁免或 latest=current 修复选项；失败输入不变。

**必要分阶段验证顺序：**

1. inspector 验证完整 before。
2. 阶段 1 测试可自行构造 source-supported after task graph；未来 writer 则必须先精确命令 replay。
3. after graph 临时携带旧分类时可能不满足 terminal history，不能在 append 前要求它已经是最终有效 snapshot。
4. builder 返回新的分类候选；用原样保留的其它 supplement 组成 complete after snapshot。
5. 对完整 after 重新运行 inspector；仅这一步成功才能取得新的 validated 类型。builder 成功本身不意味着新 snapshot 有效。

### 7.3 successor checker

概念接口：`checkForwardHistorySuccessor(previousValidated,nextValidated) error`。

- 两者都已独立通过同一固定 profile。
- previous 所有 history keys 保留；每条旧 stream 为新 stream 的逐项 canonical/bit-exact 前缀；旧 IDs、内容、revision 不变。
- classification revision 增量恰好等于新增 events 数；不能删除旧事件后重新编号。
- 原 day identity/date 保留；原非 nil lockedAt 不变；不把整日其它字段冻结成未来领域公理。
- 失败不改任何输入。它不批准 graph delta、不证明动作授权、不替代 replay、不保存资料。

阶段 1 不把该 checker 接到 store。未来 store 调用且故障矩阵验收前，不得称跨 revision 持久化保护已上线。

## 8. 阶段 1 的合成测试与实际准入

本轮未运行以下测试，它们是拟实施规格。纯候选测试须说明 fixture 的来源类别：

- `Service-profile compatible payload`：现有一次 capture 合法 payload 可由候选 inspector 验证；不表示旧 v2 session 可继续使用。
- `source-supported synthetic read extension`：open-U 及其更晚锁日；不声称 Service reachable。
- `profile-unsupported source output`：局部 core 时钟合法但违反 forward profile 的 A@10→Lock@5。
- `malformed / necessary-condition violation`：直接篡改字段/事件的负例。

### 8.1 最低矩阵

| 组 | 规格 |
| --- | --- |
| 既有 payload | C/U/R/D 单 capture；空 P/C；无历史 future X；候选通过不替换旧 reader |
| 多事件 | P→A→P→A；U→A→U；P/latest A；open C/latest A；A→P→C→lock；A→P→R/D |
| 锁状态负例 | UAA/UAC/UAD/UAR、UU、C/R/D 后 event、locked P；避免只测邻接 |
| R1 profile | 拒绝 `[A@10],L=5` 为 unsupported profile；接受 `[U@5,A@10],L=5`；接受 `[A@10],L=20` |
| open-U 扩展 | A@10→open U@20 保留 settled=10；更晚 Lock@30 无第二个 U；明确只是纯候选 |
| R2 completion | 锁日 `[A@10,C@20]` 配 completedAt=5/content=15 拒绝；实际 complete@15/Lock@20 正例；open C 对应测试 |
| R3 日期 | min day open A→U 与锁日在 Today=min+1 可解释；max day open A→U、任何 Lock 拒绝；max day open A→P/未锁 complete 可解释；min day future X 拒绝 |
| 保证上限 | `Ux@10,Az@10,Uy@10` 相同 settlement time 的构造示范可通过必要检查但不可标 Service reachable；测试注释保持该边界 |
| 全局结构 | UUID 大小写冲突、重复/缺口/越界 revision、key/trace mismatch、stale latest、空流、dangling、scope 非空、unsupported events |
| 时间/bits | +0/-0、负有限、小数、子正规、极值、NaN/Infinity、capture 下降；同值不同 bits 赋值关系；相等事件时的有限分支 |
| builder | 已有流追加与最新替换；新增 key；零 intents；预分配 UUID 冲突；checked overflow/容量；固定输入固定输出；失败无修改 |
| 分阶段组合 | before valid；after+旧分类暂时 invalid；append 后完整 after 才验证；builder 返回值不能伪装 validated |
| successor | 两端单独 valid 但旧 event 改写/删 key/重编号/改 L，拒绝；合法追加通过；不授权任意 graph delta |
| 别名与上限 | bounded Inspect roundtrip；完整 supplement 篡改；输入/返回深复制；map/slice/pointer 不泄漏；最大允许节点/事件计数；状态槽≤84、ε轮次≤84；不同bit token不错误合并；final completedAt约束不丢失 |

### 8.2 后续 clock collector 的测试措辞

将来按 Swift `MutationClock.swift` 补全统一 frontier：所有 events/latest、task clocks、terminal、plannedSubtask、note tombstone、preferences 等。当前阶段不改 Service clock collector。

字段提取器单位测试应逐项验证收集；完整合法 snapshot 检查最大值或**并列** frontier。不能要求每个字段在合法 aggregate 独自最高：terminal≤content，note.DeletedAt=updatedAt，plannedSubtask.CreatedAt≤definition.contentUpdatedAt。不能制造 invalid graph 后声称已覆盖合法 Service 极值。

### 8.3 阶段 1 准入/退出

拟新增内部纯模块与其测试；无需新运行会话、数据库或旧会话读取。先独立审查状态机/profile/API，再实施 pure inspector/builder/checker；完成 focused/race/vet 和旧 reader 未改变的回归后再独立复核。

工具资源沿用固定 Go 1.27.3、显式外置 BUILD/GOTMPDIR/logs、已有 cache、低并行、GOPROXY/GOSUMDB=off。不能复制工具链/缓存、清缓存、运行重型 release。未跑 Swift oracle或非 Linux 实机不得写成通过。

阶段 1 的成功只能描述为“未接线纯审计模块通过合成验证”；不能说 Abandon/Reactivate Service、reader 扩容、磁盘保护、UI 或全量 Swift 对等已完成。

## 9. 后续接线路线：不是阶段 1 的实施范围

### 9.1 typed settlement/completion 前置

- 新增专用 typed settlement replay，输入含 Today/Now；完整 prior/before/after 绑定，独立精确重放 SettleTaskDays。
- 仅 before 未锁日 P→after U 与 before C→after C 生成 capture。已 U/A/R/D 无新 capture。不能只根据 after U/C 选捕获。
- complete/undo 使用明确 typed input 审计，旧 stream/latest 原样保留；final completedAt 来自命令实际 Now。
- `CaptureEmptyHistory` 的 generic locked/terminal 冻结不加通配例外；generic 不得因 inspector 能读 A/P mismatch 就直接改变旧 A 或 chain state。

### 9.2 existing return/defer 与新 history-chain writer

- current return/defer 对已有 stream 用共同 builder；不能 append 重复 map key。
- continue 不 capture 源，也不把源 classification history 复制给目标。
- future A→reactive P→X：core task facts 可能成功，完整 snapshot 在 aggregate 层拒绝 X 带历史；不删历史、不改为 R、不改日期，reservation 前失败。
- 被后继引用的 U、没有 pending 可重定向时 abandon 仍由 core topology 拒绝；不补改后继。
- 新 Service API 未来可为 `AbandonChain(chain,requestedTrace) error` 和 `ReactivateAbandonedChain(chain,trace) (sameTraceID,error)`；abandon acknowledgement 若含 ID，必须明确 affected 与 requested，不用请求 ID 冒充目标。
- 每次同一写锁内检查 session/blocked，绑定实际 s.today，取一次统一 Now，独立 resolve actual target，再以相同 core input 精确重放 whole graph。
- abandon 只对 actual target 追加 A；reactive U 只对 source 追加 U；reactive P 零 captures，整个旧 classifications 不变。
- 新 UUID 在上层准备后交 builder；组合完整 candidate，inspector、scope、codec normalize 和精确预期对账通过后才 commit；确认 commit 后才发布 graph/snapshot/ID。
- 其它 chain create/schedule、带旧 A stream 的 P 做 title/manual/subtask 都须兼容回归，不只测试新入口。

### 9.3 AdvanceTo、时钟及原子边界

Open/AdvanceTo 先 reconcile，成功后才发布 Today。不能在新命令里先独立提交 AdvanceTo，再宣称复合动作原子。若未来需要 settle→command 单事务，另设计 typed composite command，完整中间候选/意图与一次 commit 都须可审计；不在当前范围。

自然日倒退不解除日锁；locked reactive 仍 U。未来 mutation clock 取全部持久化 frontier 的 next representable instant，即使 wall 倒退。probe 不 nextUp；MaxFloat64 下真实 no-op 可只 verifyPrefix，真实 mutation 无法前进则失败。

### 9.4 磁盘失败与并发

保持既有规则：reservation 前失败不发布；另一 handle 抢占返回 conflict；reservation 后任何不确定结果保持 blocked，不删除残留、不自动重试。after-persist/verify 失败可能已写盘，返回失败/空 ID 不等于磁盘未变；fresh Open 确认后再决定，不重复 append 补偿。

所有 record/prefix、COMMITTED、hash、marker、目录/容量/类型限制不放宽。旧 session 损坏不 repair。Windows directory fsync 限制与断电持久性不升级成 production guarantee。

未来再做双 handle、重复 abandon/reactivate、AdvanceTo 交错、Snapshot race、fault stages、stale ack 和完整 reopen 测试；阶段 1 纯测试不能替代这些。

## 10. v3 clean cut、风险与回滚

### 10.1 固定兼容策略

阶段 1 不改 marker/record，不碰旧会话。

未来 writer 接线时，只在**全新 development session v3 marker/record format**下开启新历史语义，wire snapshot 保持 v7。旧 v2 原样保留，不迁移、不覆盖、不删除、不在 Open 时改写、不原地续写。新 binary 可对 v2 返回明确“请新建 v3”的 typed reason；这不是迁移失败后的 fallback，而是既定 clean cut。

“候选 inspector 能读旧合法一次 capture payload”仅为纯数据结构兼容测试，不表示新 binary 会打开旧 v2 session 继续使用。

旧 binary 应在 v3 marker gate、任何 reconcile 前拒绝；后续必须以旧 binary 黑盒证明无写。不得删新 history、清 revision 或改 hash 来迎合旧 reader。

### 10.2 分阶段风险与回滚

| 阶段 | 风险控制 | 回滚 |
| --- | --- | --- |
| 当前文档 | R1保留、R2冻结、独立复核；无实现 | 不采用 R2；不改 repo |
| 未接线纯模块 | 固定 profile、private validated 类型、bounded tests、旧调用零变化 | 撤下候选文件，不影响会话 |
| typed replay 候选 | 先独立实现/验证，不激活 writer；generic冻结不放宽 | 撤下未接线候选，不改历史 |
| 新 v3 writer | 全新隔离会话、完整故障/并发/codec/prefix验收 | 停止 writer，保留全部 v3，使用兼容 binary；必要时新建独立旧版合成会话，不降级原资料 |

不新增 production 流量或监控。后续隔离验收只记录 typed stage/reason、事件/record 数与耗时，不记录任务正文、真实路径或用户资料。

## 11. R2 对审查阻断项的处置

- **R1 clock profile：已收口。** §4 明确 fixed forward-clock；A@10→Lock@5 标为 source-supported but profile-unsupported；open-U 明确只读合成扩展。没有修改 core 或宣称现有 Service bug。
- **R2 completedAt：已补齐。** §5.4/§6.2 强制 final complete witness=actual completedAt；locked/open C 都要求末 A.capture≤completedAt；§6.3 给出直接正反例，不新增 content==completed 通用公理。
- **R3 保证与日期：已收口。** §3.1 给跨流批次反例；§5.3 加合法 Today 存在条件及最大/最小日期；§7 明确三纯职责与 append 后完整验证的阶段，不将 builder 当 aggregate 验证器。
- **接口/范围补充：已落实。** event ID 上层预分配、纯 builder 确定性；没有 Service/旧 reader/store/UI 接线；未来 v3 clean cut 已确定，不再列用户决定点。
- **clock 测试措辞：已修正。** 逐字段提取测试与合法 aggregate 最大/并列 frontier 分开，不用无效图填覆盖率。
- **输入/工作量边界：已补明。** 使用现有bounded v7.Inspect roundtrip取得独立完整对象；状态/lock/token/final completion封闭键至多84槽，有限ε闭包和扫描上界明确，不以超时/路径截断代替正确性。

R2 的原三项阻断已获定向复核关闭；当前 R3 只完成终态 token 表达的精确勘误，仍须 reviewer 对本次冻结差异确认。不把设计复核写成实现或功能完成。
