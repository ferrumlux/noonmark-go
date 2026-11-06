归档说明：以下为本批设计或隔离验证的阶段记录，其中待实施／待整合措辞指当时状态。原文 SHA-256：`aedb79b4fb0c5c96394b005991a7f54de41e21835b972e1a77c1cbea396fd124`。归档仅调整协作措辞与任务标识；正式整合结果另见本目录 README。

# Typed history command audit R2 设计复核

task-id: review-history-command-audit

日期：2026-11-06。结论：**Accepted R2，原唯一阻断 R1 已解决，无新增设计阻断。** 仅批准该冻结设计进入后续隔离实施与独立验证，不是实现、资源安全、并发或 Service 接线验收。

## 冻结对象

- R2：`/workspace/shared/noonmark-history-command-audit-design-20261106/revision2/DESIGN.md`，实读 SHA-256 `d8d0391913f3aea0f7bb5dec391654624b0fe72785abf23797b514d9b21a3f2d`。
- 原 R1 设计仍为 `aa9ad33f933e37bad9c39a8b9a6f33012a5ad8db431458465dcf1628f6f73dbf`；本目录原 `REVIEW.md` 保留不改。
- 正式 HEAD `03a18cca51766e1d6b501e5e015ce96d0be5a7c7`、tree `b85d63244f6352b4e0b6781d40f48fd50343ad08`，本轮检查 status 无输出。
- 本次读取 R2 与差异，并重新逐项核对 typed model、v7 schema/encoding、canonical writer、builder 的资源常量。**只有源码及结构证据，没有实施、编译、测试或性能运行。** 无正式树/作者文件修改、远端操作或真实资料访问。

## R1 的闭环核对

1. **入口顺序明确。** R2 §5 第1–3步只先检查封闭 prior，然后把 before、after 和输入 IDs 全部 preflight；通过前禁止任一 caller graph 的 Validate、FromDomain、Clone、内容扫描、索引及按输入长度分配。该顺序正好阻断原例在 `FromDomain→State.Validate→indexBy` 内先做无界工作的问题，不需要修改既有 core/codec/r1。
2. **节点成本正确。** 对照 `domain-codec/v7/schema.go:33–44`、`domain.go:109–129,180–198` 及 `domain-core/core/models.go:110–194`，七种基础成本确为 Day=17、Chain=13、Definition=19、Trace=33、Subtask=25、Note=11、PlannedSubtask=17；五个顶层 array 从5起算正确。每个标量字段=2、wrapped ID=4、LocalDate=8；optional Membership=14、DraftCancelledOn=8、draftCancellationID 标量=2，表中其它 optional 亦齐全。空 nested array 已计在父基础成本，子项追加不重复计数组。
3. **前置工作自身有界。** 五个顶层 len 先 checked 预扣；每个 nested slice 在进入子项循环前预扣；optional nil 检查与 string header 长度遍历只在已界定记录中进行。`sum<=limit` 后减法及 `count<=(limit-sum)/cost` 后乘加避免 uint64/int 溢出。未依赖构造 AST、复制或内容扫描来取得预算。
4. **所有字符串覆盖完整。** R2 §5.1.C 覆盖全部 string alias、普通/可选正文、各类 ID/optional ID、state/status 和 CycleMembership.SeriesID；没有因稍后 scope 拒绝而漏计。共享 slice、string、*string 逐逻辑出现计费，匹配 wire 的展开方式；不是按物理地址去重。typed model 无递归容器，gate 的 O(1)额外状态和有限记录遍历论证成立。
5. **不会以预检误杀合法完整输入。** task-forest 只计完整 envelope 的一部分节点；原始字符串长度是有效 UTF-8 canonical 编码的下界。`canonical/canonical.go:261–296` 的字符转义只增不减，未使用6倍最坏扩张上界来拒绝。before/after 各自预算，未合并成一个 N/B。最终完整 envelope 的精确 bytes/nodes/depth gate 仍必需，metadata 通过本身不作完整接受。
6. **IDs 的提前限制合理。** 每次 settlement 每 trace 至多 capture 一次，故从 sealed prior 取 trace count 作为上界不会丢合法命令；N/15 与现有 builder `history_vnext_builder.go:36–40` 一致，B/36 是独立原始字节上界。先 count、再逐项 len==36，之后才允许字符/大小写/hash/copy；completion 两个身份同样先查长度。实际 capture 精确数量、合法内容、全局冲突和 builder checked revision/容量全部保留。
7. **实施阶段可验证。** §5.1 和 §9.1 已要求预算/领域同时无效的入口反例、固定 typed stage/reason、零结果/输入不变、两个 graphs/IDs 顺序覆盖、AST 前置返回门禁与调用链审查，且明确 stage error 单独不是控制流证明。字段反射清单、optional 成本对账、共享叶片、临界/溢出、合法完整输入正例足以约束实施方向；不依赖生产 callback、OOM 或超时来证明拒绝顺序。这些门禁仍须真正实施和运行，本次不声称它们已通过。

## 其它修订与范围

- ID round-trip 已保留原判断并增加对照用例：task rawValue 不能精确回组则拒绝；不能因此改写或一概拒绝可原样保留的旧 classification event/key 大小写。没有收紧当前受限 writer 的既有资格。
- 已核对整合阶段单侧 Go 观测的 `observation.json`、日志及三个文件 hash：probe `2b983fc03295d03847e7f03a65ab427c56930656a78d79b882d3c38f61c9e21a`，overlay `0954262ee9b38377d5f7d6d62dea57f5b97c6111cce6cb4092ee51c02fcb2c37`，log `c6766c0b8bd9d7aac19a2a8c2a8df1f8258b3b9f8ae7d86dad1bf5632c76f803`。R2 对 complete/undo 的 ErrInvalidClock/不变结论准确引用该 Go 观测；明确合成 chain clock 手改、非 Service 来源、Swift 未运行。**该运行不是本复核执行，也不是跨引擎 red/green 或新 helper 验证。**
- 从 R1 到 R2 的差异没有扩张两个 typed 入口、封闭结果、整图 replay、内部 capture、完整 after inspector、prefix、P→U/C 新锁捕获、已有 U/A/R/D silent lock、X-only no-op、旧 A 保留、Go 排序/Swift Set 边界或 clock 分层。
- accepted r1、generic 冻锁、Service/store/UI、v2、v7 wire 保持不动；未来 v3 writer/full frontier/Swift 差分仍为独立阶段。完整实现须按 R2 的作者验证、独立审查及整合门禁取得精确树证据。
