# 普通单分组隔离候选

本包按固定上游 `43e6eefaae3a04be89cab3e234d470111afd80ef` 的 TaskClassification.swift／TaskClassificationStateValidation.swift，以及已签收 R3 设计（SHA256 `ece8bbde72d713676e87f02b9d72d6ff6674cda065713a1ba8c67aa515e23744`）实现候选。只用于 development/e2e 合成资料。

## 范围与证明层级

- 只支持 userDirect、普通固定 task graph、每 chain 至多一个分组、none／existing／new 的 setCurrent。创建并加入只有一个 record／revision；新 interaction 的相同选择仍推进 revision，精确原 plan 重试不新增事实。
- labels、history capture、管理操作、AI、周期任务和 supplement 不属于本包。旧 classification.Engine、taskapp Service／store／UI admission 完全不接入本包。离线 v7 adapter 不注册 AuditValidator。
- NewEmptyContext 在完整预算预检后验证并复制普通 graph；拒绝任何 CycleMembership。它不证明任务生命周期，也不增加 active／pending／日期限制。
- ValidateApplyIntegrity 检查受限形状、名称版本及归属、记录／receipt 双向绑定、关系 origin／removal 与完整关系覆盖。它不调用实体 Codable decoder，也不证明 catalog 创建来自保留计划。
- ValidateDecodedEntities 额外执行实体名称固定点、key/version、单开放初始 NameVersion 与时钟要求。Swift String 等价用固定 NFC 比较，输入字节不被改写。
- Replay 只能从空分类起点重放完整 typed transcript。Continue 只能延续本包密封的完整证明。裸 imported state、hash、revision 或 success bool 均不能充当 proof。返回 state、plan、transcript 和 graph 都是独立副本。

## 调用与身份

Prepare 接收原始 Request 与显式 PrepareIDs；ConfirmUser 返回绑定该 sealed Plan 的确认；Apply 接收 CommitIDs 和原始 core.Instant bits。显式 ID 槽不会读取随机源，也不会在失败时消耗外部 ID provider。未用的预留槽忽略，不产生事实。decisionID 可以等于 interactionID；各角色不人为全局互斥。

重试顺序是预算／受限形状／原计划／确认／digest，再匹配 receipt+record，然后才检查 stale 与重算当前 preview。重试不读取新的 now，NaN／Inf 也不改变已提交事实。新 mutation 拒绝非有限时钟、相关当前关系倒退及 revision overflow；不添加全局提交时间递增 gate。

名称沿原版 D/K 阶段：prepare 保留 D(raw)，resolve 再 D，实体 initializer 再 D，最终 name=D³(raw)，key=K(name)。UI 若在进入本包前先 D，则 name=D⁴(raw)。existing 不改名称，new 重名转 existing 并保留 reuse notice。单分组创建不是旧 Engine 的 Record 改名。

## 资源边界

固定上限镜像 canonical.DefaultLimits：16 MiB、1,000,000 logical nodes、depth 128。它们是保守的累计资源子集，不是精确 RSS 上限，也不承诺任意 16 MiB 名称／完整大小 catalog 可用。

每次入口先按实际 graph、state、请求与完整 transcript 计数，包含 key、别名每次引用和全部字符串字节；此后才检查 UTF-8／UUID。输出、clone、索引、预览、record／receipt、重复重放与实际 digest 编码共用一个账本。负配置、超硬上限与 checked arithmetic 溢出均拒绝。

每次 D、K、NFC 调用分别先收费；Equivalent 为两次已收费 NFC。K 内含的 D、输出、rune 展开、排序暂存也计费。固定 UCD16 的 18×4×4=288 与 4×4=16 上界由 TestFixedUnicode16ExpansionBounds 对两份 SHA 固定 UCD 独立重算。已生成的输出先验证长度不超过预留上界，其实际 byte 长度是下一次调用的更紧输入上界，不能把 key 送入未计费的 K。digest writer 在预留的固定 capacity 内直接输出 bytes，不经默认 Go JSON 或 v7 serializer，不额外复制 String→[]byte。

ImportCandidateWithLimits 让离线 adapter 在扣除完整 wire 输入／转换成本后，使用单一剩余账本完成 graph+state 校验与复制。ExportCandidate 预留完整 owned 输出、固定 neutral supplement、6×字符串转义及 wire wrapper 节点成本，再返回 graph+state；它只输出可 decoder round-trip 的候选。

## 验证状态

本批代码和测试最初仅通过 gofmt／静态 diff 检查；运行结果以外置证据记录为准。Go 自洽测试、原版 Swift 实际 capture 和产品接线验收分别报告，不互相替代。

Foundation digest writer 明确限定固定 ASCII keys、sorted order、optional omission、enum wrapper、UUID 字串与 raw Instant UInt64 bit pattern。没有真实 Swift payload 对账前，不能称 wire compatibility 或跨引擎语义已通过。原有 1807 项名称任务分母保持不变，Go 产品无 Swift 运行时依赖。

主动 fuzz 需要另外限时执行，普通 test／scripts/check 仅执行 seeds：

- `go test ./classification/ordinary -run='^$' -fuzz='^FuzzOrdinaryReplayDoesNotMutateInputs$' -fuzztime=30s -parallel=2`
- `go test ./classification/ordinary -run='^$' -fuzz='^FuzzOrdinaryAuditRejectsTampering$' -fuzztime=30s -parallel=2`
- codec 模块的 `FuzzOrdinaryAdapterBoundedRoundTrip` 独立执行。

所有缓存、二进制和证据必须外置，不在 source 建立 .build、artifacts 或 dist。

## 独立审查修复 M2-EQ

R3 初版把 Apply 的 expectedChanges 校验写成重新 prepare 整个 plan，并比较新算出的 digest。已通过实体 decode/integrity 的 category 名称或颜色若只有规范等价的字节表示差异，会因此被错误拒绝。固定上游 TC:2327–2342 在原 plan digest 已验证后，仅以 Swift Equatable 检查当前 expectedChanges 与 blockers。

现已拆出 validateExpectedCurrent：使用原 sealed prepared intent／planned IDs，只重算当前 before／after preview；kind、ID、chain、resolution 精确相等，name／color 用共用预算账本预付两端 NFC 后比较。不会重算或比较另一个 plan digest，也不会改写原 plan、record、receipt 或导入 state 字节。原 samePlan／sameChanges／sameReceipt 的字节绑定保留给 Replay／retry；保留空切片的逻辑等价，不扩成全局名称等价。真正创建前继续拒绝已占用 planned ID。

TestExpectedChangesCanonicalEquivalencePreservesOriginalFacts 的五个 case 在 R3 先真实判红，修正后转绿；另覆盖不同名称／颜色／身份／resolution／chain、双端累计预算、原 retry／transcript 字节绑定与创建身份冲突。R4 ordinary race／vet已通过；此前主动 fuzz 的结果仍绑定 R3，不能冒称已在 R4 重跑。独立复测及最终集成检查按外置证据分别签收。
