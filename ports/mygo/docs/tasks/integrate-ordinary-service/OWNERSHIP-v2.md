# 活 Service 磁盘所有权合同 v2

## 从旧合同到明确扩展

本合同在后端组合 `742dd0bc3476833fce00b489c30df12ed9f0e146` 的基础上，明确扩展[原三维回执合同](../service-typed-outcome/IMPLEMENTATION.md)的“仅两个 outcome wrapper”限制。旧文档、旧源码清单与失败记录保留为历史。本版是架构合同扩展，不是把旧测试当作过时字符串后直接放宽。

首次完整 `scripts/check` 在该基础版本上真实失败：`TestOutcomeRawDiskOwnership` 检测到 ordinary Service 新增调用；root package 已通过，taskapp 因此失败。该次运行不算完整通过，也没有继续 cross-build 或 entrypoints。本合同新增有限检查不改写那次结果。

v2 只允许下列三个精确活 Service 调用，路径、所属函数、callee、次数均固定：

1. `outcome.go:commitLocked:s.disk.commitObserved`，一次。
2. `outcome.go:verifyPrefixLocked:s.disk.verifyPrefixObserved`，一次。
3. `ordinary_service.go:persistOrdinaryLocked:s.disk.commitOrdinaryObserved`，一次。

前两项及原全文件扫描、计数、精确集合比较全部保留。新增项只接收私有 `persistableOrdinaryCandidate`，不是任意 snapshot、bytes、validator 或写入回调。Create/Open 未公开前、store 内部及私有测试的原边界不因本版扩大。本版没有授予任意模块自设第四个 owner 的许可。

## 第三 owner 的完整条件

公开调用持真实 Service 锁。persist 在写前检查健康、恰一 task/category edge、before、owner、head、revision；Facts/Snapshot 失败须在写前返回。writer 独立核验真实 Service 与 disk 的双向绑定、独立 writer head、健康、edge、完整 before/owner/head/revision；从 before 重放封闭 edge，并检查 expected bytes 及 after.document 编码两者都匹配候选 bytes。所有候选错误在 reserve 之前处理。

实际 writer observation 必须传入 `setAttempt`；invalidates 必须使用实际 reason/stage 停用 Service。writer error 必须先返回，之后才能发布 aggregate/graph/snapshot。aggregate 只能来自已确认的 `disk.ordinaryHead`。disk revision/hash/head 仅在最终 readback 成功后推进；不可把裸 candidate.after 当成确认。

observation 的封装现在分布于 outcome 与 ordinary Service 两处，维护者必须同时保留这两处合同。采用此有限扩展是为了保持已审产品字节，不表示分散封装没有维护成本。

## 实际检查方式及局限

`ordinary_owner_contract_test.go` 的检查器接收源码 AST 参数，核对全部调用集合，并对五个关键函数的签名及每个顶层语句的规范化 AST 做严格匹配。每个语句包含完整嵌套分支，因此条件、return、observation 参数、错误支配发布以及先后顺序都会参与匹配；不是只搜索函数名是否出现。固定段落和摘要在 `internal/taskapp/testdata/ordinary_owner_contract_v2.json`，来源为上述固定版本；不能从当前被测源码即时重建 expected。

五个函数是 `persistOrdinaryLocked`、`verifyOrdinaryCandidateLocked`、`commitOrdinaryObserved`、`validateOrdinaryWriteInput`、`commitStoreInputObserved`。失败诊断指向函数与控制流段落。未来合法重构也会被拒绝，须重新审查及明确修订合同，不能自动刷新摘要。

合成突变直接传给同一检查器，先确认语法可解析；不以编译失败充当 kill。覆盖删除／交换语句、各层条件反转、删除拒绝分支、不可达检查、额外／重复调用、错误位置、method value／receiver alias、丢 observation、错误 attempt／invalidation、错误或提前发布、裸 candidate 发布。独立审查还需另写突变与正控制，不能把作者负例称为独立证明。

真实合成 Service 补充检查 closed／needs-reopen、坏 owner/before/head/revision/edge 在 reserve 前拒绝且磁盘与公开状态不变；before-reserve、after-reserve、after-persist 的 attempt/health、不发布与后续门禁；成功时 aggregate 与 writer head 同指针，完整 ReadView 可重开恢复。

原全文件 scanner 只识别字面 `s.disk.*`，不是完整 Go alias／dataflow 分析。第三 owner 的固定 AST 结构可拒绝该函数内改名、alias、method value 与不可达诱饵；**不宣称全库任意别名绕行均被识别**。产品文件仍逐字不改，默认空 catalog／UI 入口不开放。此版本没有新增完整门禁、Swift、native、UI/IME 或生产验收声明。
