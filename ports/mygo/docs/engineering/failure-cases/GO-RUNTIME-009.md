# GO-RUNTIME-009：回池终态审计与新身份边界

- 状态：处理中；独立反例已重现，独立审查的修正后 race／vet 已通过，正式集成待完成，未指定修复 commit。
- 首次发现：2026-11-06（新加坡），普通任务回池候选独立审查。
- 影响：隔离 development 的直接 capture/kernel 调用边界。没有生产资料操作或已知用户数据损坏。正常 Service 随机产生身份可降低碰撞概率，但不能取代 kernel 的身份契约。
- 引入提交：generic capture 只冻结锁日定义的逻辑来自 `501fdfca858a6c437dca4875aaca4bd260627fbf`；Git author／committer 均为 ferrumlux。真实操作者无法仅凭 Git identity 确认。回池前未锁终态及身份问题来自本批尚未提交的候选，修改者为本轮实施 agent，未伪造 commit 归因。

## 原始症状与反例

1. pending trace 保持不变时，调用 generic CaptureEmptyHistory 可以新增任意 successor 并 supersede 当前定义。
2. 今天回池但 day 尚未锁定时，generic capture 可以重写 returned trace 所引用旧定义的标题。轨迹已是历史事实，不应等待锁日才冻结其内容。扩展反例亦证明今天与未来回池的实际子任务在未锁日可被 generic 改写，已一同冻结。
3. 未来回池输入复用一个已取消子任务的 cancellation ID；隐藏子任务准备阶段先清掉旧 ID，使末尾图验证无法看到碰撞。
4. 新计划子任务可以复用旧定义内的计划子任务 ID。结果图验证按定义检查身份，不足以证明命令输入是新身份。

独立审查在隔离副本执行 `TestReviewerGenericRejectsArbitrarySupersession`、`TestReviewerGenericRejectsReturnedDefinitionRewrite`、`TestReviewerReturnCancellationMustBeFreshBeforeNormalization`、`TestReviewerReturnPlannedIdentityMustBeFresh`，全部在相应旧候选判红。原反例完整纳入 `return_audit_review_test.go` 和 `return_identity_review_test.go`，没有删除断言或改宽阈值。

## 根因与修正

generic capture 的历史判断把「锁日」误当作唯一冻结边界，而且未限制 generic 命令词汇不能做定义版本化。现在所有旧定义均冻结，旧 chain 不可经 generic 新增定义；标题和回池必须由各自精确重放证明。既有 returned/cancelled traces 及其实际子任务在未锁日也冻结，日结仍可按规则锁 day。

kernel 末尾图验证只验证结果，不证明输入身份的来源。现在 cancellation ID 在清理前对照所有旧 trace/subtask cancellation IDs；计划子任务 ID 对照所有旧定义以及本次已使用集合。错误输入在深复制事务内拒绝，不发布部分修改。

排除了“Service 使用随机 ID 就无须验证”“terminal 等同于日已锁”“一个合法 after graph 就代表合法命令”等假设。没有放宽 codec、修复缺失审计、删除不完整会话或绕过 exclusive-create。

## 门禁、发行与回滚

四个原反例为 fast gates；真实 Service 的 `TestReturnCurrentFutureReopenAndSettle`、`TestReturnWriteFailuresNeverPublish` 和 `TestReturnAuditRejectsForgedCandidateAndMissingEvent` 为本批持久化症状 gates，全部由 `scripts/check` 强制执行。此批未增加 UI，故这些不是桌面菜单／OS 交互验收。

修复只在候选源码，不能提前标记生产发布或 UI 对等。回滚只回滚本批源码；wire 仍为 v2，但旧 reader 会 fail-closed 拒绝新增回池／取消事实，不能承诺旧版可继续打开新会话。不得删除、转换、覆盖或自动修补历史会话来迎合旧 reader。集成时补记修复 commit，并保留独立红→绿证据。今后新增 terminal 状态要同时审查锁前冻结、精确命令重放与身份清理前的碰撞证据。
