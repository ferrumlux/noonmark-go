归档说明：以下为冻结设计或独立阶段记录。状态和路径指当时阶段；原文 SHA-256 为 `e38c0b89488d2fbf7561e441ffc4ed0055406f71865f8ac20f877ea8c5664f6f`。整合进度以本目录 README 为准。

# Lifecycle history 设计独立复核

日期：2026-11-06。

## 裁定

**Accepted：只准进入隔离实施。** 未发现需要改变 fixed profile、core lifecycle 语义或 capture 合同的阻断项。本裁定不是实现验收、Swift oracle／平台对等、持久事务或 v3 writer 接线许可。

本轮仅源码读取、哈希与 Git 状态核对；没有运行测试、构建、App、数据库、网络或 Git 写操作。只在本新 review 目录写入报告与哈希。原上游未改动。遵循 port 的隔离授权，取代上游针对另一台机器的历史固定目录要求；production／iCloud／Keychain／live AI 禁区不变。

## 冻结身份及范围解释

- 已核实正式树 clean，HEAD `3ea3efeaee2a0085576ca1aadd0d3082e643a558`，tree `b7097068ebf44c167b6df8ac9162261946ea3bfb`。
- 上游 HEAD `43e6eefaae3a04be89cab3e234d470111afd80ef`。
- DESIGN SHA-256 `487f317195ed105f308dd9da6f71def2a094b9336e2901af81669f1293b7005e`。
- TEST-ORACLE SHA-256 `16bec4700f9685b7be9a1ff6e09218177596044445237e5925e1949207eeff77`。
- 作者 `SOURCES.sha256` 全部条目已核对通过，包括七个既有 command 文件及 future-cancel 附录。附录仅作为源码导航，不作为已执行 oracle。
- 已读上游和 port AGENTS、领域词汇及相关 ADR-0004/0020/0022/0027/0037、port DESIGN 与相关实际源码。

整合任务最初要求“七个旧 command 文件 byte-exact”，与冻结设计明确扩充 result 和 AST tests 冲突。复核中整合任务已明确纠正：**原 r1 inspector／builder／machine、core／profile／runtime／v2 冻结；准许审查设计已声明的最小 command result 与 budget-test 扩充。** 因此不以已撤回的过宽要求否决设计。

现有 `history_command_vnext.go:36–62` 的 kind／proof／valid 是闭合返回类型必经处。新增两个 kind、两个标量和显式穷举五种 kind 是合理最小变化，不必另建平行 proof 体系。旧三个 kind 的值、构造行为、返回 copy 和 changed 合同必须保留；旧 audit 函数主体、bind/match/finish 无需重构。AST tests 必须新增覆盖新文件／入口，不能以覆盖旧入口代替。生产预算可复用现有图预算，新 ID gate 尽量留在新 lifecycle 文件中。

## 独立来源核对

以下 Go 路径相对 `noonmark-go/ports/mygo/`；Swift 路径相对 `noonmark-reference/`。

### Abandon 实际目标与 owner fence

Swift `Sources/NoonmarkCore/NoonmarkEngine.swift:2513–2533` 先读取请求 trace 与其 chain，再选择该链 pending trace（否则请求 trace）；仅 P/U 可废弃，最后对**实际改变的 trace** capture。Go `domain-core/core/history_chain.go:26–54,108–147` 增加显式 ChainID owner 核对且核对在重定向前，active 只选择 P 而非 U。设计分别保留请求身份和 effectiveTrace，并从 owned prior 解析目标，再对照 typed replay，符合实际行为。

唯一新 A 必须以 Now 同时绑定 event、目标 contentUpdatedAt、settledAt，清 pin，改变 owner chain；无关 trace（包括被重定向的历史请求 trace）、days、definitions、subtasks 不得变化。整图精确 match 加固定 fence 已覆盖，而非以允许字段名豁免差异。

### Reactivate 的 0／1 capture

Swift Engine `:2537–2571` 和 Go `history_chain.go:61–105` 都原位使用请求身份，不重定向：锁日优先 U；open 且 date>=Today 为 P；open past 为 U。P 清 settledAt、零 capture；U 保留旧 A settledAt，Swift `:2568–2570` 明确追加 U capture。所有分支清 pin，更新 source／chain clock，并返回原 trace ID。

Swift `TaskClassification.swift:4022–4056` 的 capture 向旧 stream append、替换 latest、revision+1，不是清理旧 A。Go fixed inspector `internal/taskapp/history_vnext.go:247–260` 要求 U tail，且区分锁日 settlement 与紧前 A 的 settledAt 见证；`history_vnext_machine.go` 的 event 分支接受 A→U，而 epsilon closure 不提供静默 A→U。因此设计的 U 单 capture 与 P 零 capture 均有完整来源支撑。

### 完整状态、表示与旧事件

`history_command_vnext.go` 的 bind 用原 supplement 分别比较 before 与 prior round-trip；match 比较 replay 和 after 的完整 wire；finish 固定调用 append、compose、完整 inspector、prefix、supplement/count fence。新入口复用这一完整管道正确，不能只使用 builder 返回值。

`domain-codec/v7/domain.go:22–60` 的 FromDomain 内含 Validate、Marshal、反射和 clone，故它必须位于所有 metadata gates 之后。该文件 `:231` 对任务 rawValue 解码 uppercase，既有 bind round-trip 拒绝表示变化；设计没有承诺将 lowercase task prior 修复后接受。

`history_vnext_builder.go:29–129` 在生成新事件时规范化新 UUID，但定位旧 stream 使用正规化索引，旧 key 和旧 event 留在原 canonical 值中。设计正确区分“新 event UUID 规范化”和“旧分类表示原样保留”。`checkForwardHistorySuccessor` 保护旧 event prefix 与已有 day lock；完整 replay／builder 所有权继续承担 prefix 单独不证明的整图和表示不变性。

### 资源顺序与闭合证明

设计的 prior seal→before budget→after budget→输入 ID metadata→bind 顺序成立。两张图各自独立有界，Reactivate 长度>1 在读取元素前拒绝，所有 ID len==36 之后才扫描 UUID 内容。复用 `history_command_vnext_budget.go:69–178` 的结构先行、raw-string-header 后行以及每次逻辑出现计费，避免在验证前按 caller 容量 clone／index。

当前 AST gate 在 `history_command_vnext_budget_test.go:424–535` 只解析旧文件并枚举两个旧入口；独立 gate 在 `history_command_budget_independent_test.go:364` 起同样如此。设计明确要求扩充，这不能在实施时遗漏。新 proof 仅保留不可变标量和 owned sealed endpoints；EventIDs slice 不进入结果。P 零 capture 仍确实改变 A→P，changed 必须 true。

### 时钟、未来取消、消费者

Go core clock 是 finite 与局部非回拨，完整 profile 另约束事件顺序；没有理由在 pure helper 加全图严格 frontier 或 nextUp。ADR-0022 的 Store frontier 仍属未来持久接线，不由当前证明替代。

Swift `NoonmarkSnapshot.swift:519–526` 明确禁止 cancelled trace 的分类 latest／stream。Go `history_vnext.go:268–271` 同样拒绝 X+A。future A→P→Return X 不能当作要求放宽 profile 的正例。Swift Store `NoonmarkStore+TaskMutations.swift:1009–1051` 经 candidate mutation；`NoonmarkStore+NaturalDayPersistenceImport.swift:886–971` 是先 save 后发布，故不能拿低层 Engine 成功声称持久化成功。本批 helpers 不解决旧 reader／consumer 遇 A 的拒绝，也不证明全消费者 cutover 已就绪。

## 最小文字澄清（非阻断，不改变算法）

TEST-ORACLE §3 的“请求旧 U、另一个 A、active chain／已有 pending……失败”应在具体测试中明确“另一个 A”的失败原因。**另一条 A 本身不是 core 禁止条件**：`history_chain.go:61–105` 与 Swift Engine `:2537–2571` 只检查请求 trace 的 A 状态、owner chain abandoned、无 pending 及 clock，没有“必须是最新 A”或“必须是某次先前 UI 选中的 A”规则。

精确替换 oracle 文句：“请求非 A trace、请求其他 owner 的 A、chain 非 abandoned 或已有 pending，以及 after 改动非请求 A，按真实 core／whole-graph gate 拒绝；另一条同链 A 本身不是拒绝理由，不得增加 latest-A 限制。”如果采用合成多 A prior，先完整 inspector 验证，再按实际 core+profile 判定；不得冒充 Service 可达历史。DESIGN §5.3 的原位 typed replay 规则已经正确，无需修改算法。

### 必须新增的同链多 A 参数例（源码推导，未运行）

用结构合成而非声称 API 可达的 prior：一个普通 abandoned chain、一个定义、两个 open days（2026-11-05／06），各一条无 lineage、无 child 的 A trace T1／T2；所有身份 distinct、合法 uppercase UUID。chain／definition／day／trace createdAt=I(1)，必要的 updatedAt 合法；T1 contentUpdatedAt=settledAt=I(10)，T2 对应为 I(20)，chain updatedAt=I(20)。两条 stream 各一 A，capturedAt 分别 I(10)／I(20)，全局 revisions 1／2，latest 分别等于各自 A。日期不同、priority 各为有效唯一值。I(n) 必须通过 InstantFromSeconds 构造。

先要求完整 fixed inspector 实际接纳 prior，再从这个同一 immutable prior 分别测试 Reactivate(T1, Today=2026-11-07, Now=I(30)) 和 Reactivate(T2, 同 Today／Now)。每个分支各提供唯一新 EventID，预期仅被请求 trace 变 U，settledAt 分别保留 I(10)／I(20)，追加 U@I(30)、revision=3，另一条 A 及其旧 stream 原样，chain active。两个分支独立，不能先执行 T1 再把 active chain 当成 T2 前态。

源码没有更早的“同链至多一 A”限制：core validation.go:230–310 限制的是至多一 P；fixed inspector 逐 stream 检查且机器接受每条 P→A 及目标 A→U；没有最新 A 筛选。此为预计合法的 profile 参数测试，不是执行结果或整图历史／Service 可达性主张。实施时若实际 fixture 被更早层拒绝，必须记录精确 reason、核对是否遗漏普通形状约束，再报告，而不能移除测试后添加 latest-A 规则。

## 实施验收必留门禁

1. 所有来源正例检查完整期望图、补充段、旧 prefix／key、最新事件、exact bits、revision、scalar proof 和独立返回 copies，而非只看状态或 count。
2. wrong-owner 请求在 pending 重定向前失败；合法历史请求路由到唯一 pending，A capture 绑定 effectiveTrace；同目标不同请求身份不得合并成同 proof。
3. open current／future P 零 ID、past／locked U 一 ID；U settledAt 保留 A，capturedAt/contentUpdatedAt 为 Now；遗漏 U、P 多 ID、重复／大小写冲突 ID 均失败。
4. 两图与 caller IDs 的预算优先错误、AST 实际控制流检查、nested／optional／共享叶重复计费；after 和 IDs 预算必须在 before Validate 之前完成。
5. 全图合法但恶意 delta、事件前缀改写、任务 rawValue round-trip、旧 classification 小写表示保留、original supplement、失败零结果和输入不变。
6. 旧 settlement／complete／undo 的现有独立测试全部保留并回归；unknown kind、零 proof、共享 prior、caller alias 修改／并发只读与 copies 验证。
7. signed zero／极值／局部回拨／全事件顺序、open U 首次 settlement、A→P→complete→undo 以及 future X+A 明确拒绝。core 成功但 inspector 拒绝时记录拒绝层，不修时间、不缩测试域、不放宽 profile。
8. 最终实施 diff 必须证明 r1/core/profile/runtime/v2/Service/store/codec/UI 未改变、旧三个入口主体与管道保留。先隔离实际测试和独立实现复核，再谈整合；本报告不代替任何未来运行证据。

设计最小修订：无需阻断性修订。实施说明中保留整合任务更正后的精确文件允许范围，并将上述“另一个 A”测试意图具体化即可。
