# 普通任务分类历史：未接线纯审计模块

日期：2026-11-06。task-id：connect-history-chain-service-audit。级别：P。

## 目标、来源与实施范围

用户目标为 Go + MyGo 完整跨平台复刻，原应用语义、资料安全及实际验证优先。本批只落实已独立接受的 R3 阶段 1：私有完整 inspector、确定性 append-only classification builder、纯 successor checker。不是 reader 扩容、writer/importer、磁盘保护或 Abandon/Reactivate UI 功能上线。

- 正式基线：`b8857778b990fd30dae35c24b03fb49c08c0cf12`，读取时 clean。
- 上游基线：`43e6eefaae3a04be89cab3e234d470111afd80ef`，只读。
- R3 设计 SHA-256：`e45ec63d1766dd6c74f1826a500333778cc8022797b7aa271a8b2618377839a3`。
- R3 接受报告 SHA-256：`682aac1bfcac1bc1eb09222ee6bef551ca4a6ebf6158ee0b3a8414a90c588f4a`。
- 候选源码由上述正式 commit 的 `git archive` 建立于 sibling `noonmark-go-history-audit-vnext`，不是复制工具链、缓存或真实会话。
- 只新增 `ports/mygo/internal/taskapp/history_vnext.go`、`history_vnext_machine.go`、`history_vnext_builder.go` 与 `history_vnext_test.go`。所有旧文件逐字不变。

没有改动 ValidateEmptyHistory、CaptureEmptyHistory、encode/decodeSnapshot、Open、Service、store、marker、record 或 UI；没有接任何既有调用。未来 v3 development clean cut 不在本批实施；旧 v2 运行行为保持不变。未来启用 v3 writer 时，v2 不迁移、覆盖、删除或原地续写。

## 三项私有职责

### 完整 inspector

`inspectForwardClockOrdinaryHistoryVNext(snapshot)` 先通过既有 `inspectEmptyHistory` 的 bounded canonical Marshal 与完整 v7.Inspect，拒绝 caller-built 环、深度、节点、字节越界。随后显式执行完整 runtime scope、普通 TaskGraph、neutral supplement、空 catalog 的检查，而不是只验证 event list 或依赖空分类 shortcut。

检查所有 latest/stream keys、尾项完整相等、全局大小写规范化 UUID、key/trace 引用、有限 clocks、每流 revision 顺序、全局 revision 恰为 1…N、全局 capturedAt 数值非倒退。事件容量根据 bounded 数组累加，从不按攻击者声明的 revision 分配。

返回 package 私有 `validatedHistorySnapshot`。只有成功结尾设置固定 profile seal；零值失败。内部 snapshot/index 不向 caller 暴露，`snapshotCopy()` 总是 deep-copy。这个证明只表示固定 forward-clock ordinary profile 的逐流必要条件，不是完整 Service 可达性、来源真实性或写入权限。

### 确定性 builder

`appendEmptyHistoryEvents(prior, intents)` 接收明确的 `EventID, TraceID, Status, At`，没有随机 UUID 或命令资格推断。所有意图预校验后一次复制旧分类，按旧 pair 索引追加 stream，原位替换 existing latest，仅新 key 增加 pair。零 intents 不消费 revision。

校验 ID、支持的 capture status、finite At、全局及意图内 UUID 唯一、checked revision increment 和 canonical 容量。固定 empty-label event 为 15 个 canonical nodes（含 object keys）；新 stream/latest pair 一共增加 37 nodes；已有流仅增加 15 nodes。输出整体再过 canonical Marshal 的 bytes/depth/nodes 边界。

输出是 `classificationCandidate`，不能作为 validated aggregate。它故意不检查尚未提供的 after graph。完整流程为 before inspector → caller 准备 after graph → builder 追加显式 captures → 组合完整 after snapshot → 再 inspector。临时 after+旧分类可以 invalid；builder 成功不等于最终 after 有效。

### successor checker

`checkForwardHistorySuccessor(previous, next)` 只接收两端各自通过相同固定 profile 的 proof。检查旧 keys 保留、旧 stream 的逐项 canonical/bit-exact 前缀、旧 revision/内容保留、新增计数与 revision 增量一致、旧 day ID/date 保留与旧非 nil lockedAt bit-exact 不变。

它不冻结 day 的其它字段，不审查任意 graph delta，不授予命令权限，不替代未来 typed replay 或磁盘提交。

## 有界状态机与归并依据

每条流固定最多 84 槽：7 个状态 × 2 个 lock-consumed × 3 个 settled-token 来源 × 2 个 final-completion-anchor。不是任意 witness 路径枚举。

- P/C/U/A 的 token 为 nil、最近 A 的原始 bits、stored L 的原始 bits。来源标签不同，即使数值相等也不归并。R/D/X 只用 token 0 代表 not-applicable，实际 terminal settledAt/contentUpdatedAt 另与 capture 或 cancellation witness bit-exact 核验，绝不把它当 nil。
- 每槽仅保留最早可行的数值 lower bound。固定 event index 下最近 A 和 L 的 bits 是常量；所有后续 guards 相同且只要求时间不早于 lower，因此更早 lower 支配更晚 lower。
- 完成 anchor 的 ε 边只能绑定实际 final completedAt；该 C 不再允许 undo。末 A 不得晚于 actual completedAt。不能任选另一个时间作为最终完成。
- open P↔C、open A→P、无历史 X cancellation 与 silent lock 为 ε 边。Lock 的 P→U、Lock 的 C→C capture 边消费对应 U/C event。相等 clock 保留所有可行先后次序。
- ε 循环不会降低 lower，故有效见证可去掉循环成为少于 84 边的简单路径。固定点最多 84 轮；达到上界仍变化则 fail-closed，不做超时、路径截断、限额忽略或宽松接受。
- 时序比较全部使用 Seconds/Before，绝不比较 uint64 bits。赋值来源检查仍 bit-exact，保留 +0/-0。
- date=9999-12-31 没有合法 Today>date，不能锁日或 open A→U。open A→P/complete 仍可行；X 使用实际 draftCancelledOn。

新增扫描保守复杂度为 O(E log E + (E+T)×84²)，新增索引 O(E+T)，每条扫描状态 O(84)。既有 codec/graph 验证另遵守其既有 bounded 合同，不伪称本批使整个系统线性。

## 明确不作的推论

- A@10→Lock@5 是 purecore 局部合法但本 forward-clock profile 不支持的输出，不能据此判源模型错误。
- open A→U 及其稍后 silent lock 是 source-supported synthetic 只读扩展，不称正常 Service 可达。
- 跨流 settlement 原子批次、routing、真实 Today、真实用户动作与授权无法从 snapshot 恢复。逐流通过与 prefix 通过都不填补这些证据。
- 没有新增完整 command log、hash 身份证明、磁盘保护或真实监控。

## 风险与回滚

本批最大风险是错误接受貌似相邻合法但不满足锁状态/实际时间的多事件流，及把分类候选误当写授权。以固定有限状态、独立复核、恶意 mutation、旧 reader 继续拒绝、私有类型与零接线限制控制。

回滚仅撤下新增候选文件；没有任何会话、存储格式或运行资料须回滚。若后续 writer 接线，必须另做全新 v3 会话、typed replay、未知提交、重开、双 handle、并发故障矩阵与旧 binary 无写验证。本批不替代这些工作。
