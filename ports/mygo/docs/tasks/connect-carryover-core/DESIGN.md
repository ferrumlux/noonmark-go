# 普通任务延期及延续：核心合同

基线 ddee25c8fc6f8e7ae7ee1251277654f8095cbd12；上游 43e6eefaae3a04be89cab3e234d470111afd80ef。仅新增 domain-core，不改正式目录、Service、codec、UI 或分类事实。

需求是复刻 NoonmarkEngine.swift:2152–2270 与 4087–4154：延期限今天待完成，目标未来；延续限历史或锁日未完成，目标不早于今天且晚于来源。先找 trace、检查活跃链、检查锁日、状态日期、最新正式前沿、唯一活跃 trace，再检查延续的池位置。显式 owner/日期编码为移植路由围栏，不发明源错误优先级。前沿按日期、延续序号、优先级、创建时间、ID，比 inTaskPool 的内容时钟排序不同。

新 trace 使用当前 definition，但描述和有效附言来自 source；复制完整附言身份和时间而排除墓碑，不回退池内容。手动百分比来自修改前 ProgressProjection。延期序号不变；延续加一。只复制 pending/unfinished 子项，按 position 紧凑重排，保留 lineage、标题、难度并显式连接来源。延期把来源开放子项写为 deferred；延续完整保留历史。Models.swift ContentMutationClock 禁止内容时钟倒退，touchChain 使用 max(old,now)，ensureDay 不修改已存在日期。新图仍需严格拓扑校验；没有放宽历史或导入协议。

TraceCarryoverInput 提供 ChainID、TraceID、NewTraceID、DayID、TargetDate、Today、Now 与精确 source SubtaskID→新 SubtaskID 映射。命令不生成随机 ID。DayID 仅在新日使用；其余新增身份不得复用。int 溢出以移植 ErrInvalidInput 拒绝，而非声称 Swift 存在相同可恢复错误。mutate 深拷贝、全图校验后发布，失败不留下半图。

风险：核心不持有 catalog、分类快照或事件，不能凭成功声称可持久化或 UI 已接通。后续 Service 须在同一候选事务执行 exact replay audit、延期来源分类快照/event capture，完整 supplement 校验、磁盘前缀与故障原子性验证，再发布。撤回延期、重复任务、完整 undo 不在此接口范围。回滚只移除未整合候选文件；不得删除持久事实迁就旧 reader。

验证使用命令建立正常路径；现有命令无法产生的边界只用明确标记且先通过 ValidateTaskGraph 的结构 fixture。来源测试对应 NoonmarkEngineTests 的 CurrentPendingDeferralIsDistinctFromContinuation、ContinuationKeepsHistoryAndBlocksDuplicateActiveTrace、ContinuationCopiesOnlyOpenSubtasks、ContinuationRejectsSameDayAndNonFrontierSources、ContinuedFutureTargetCanReturnToPoolWithAnExplicitPoolLocation、DeferredFutureTargetCanReturnToPoolWithoutRewritingSourceHistory；以及 SnapshotTrajectoryTopologyTests、SnapshotUndoCancellationTests。这里只执行 Go，未执行 Swift oracle。

## 恢复后的测试边界

保留 focused-1.log、focused-2.log、focused-3.log，不将早期绿色当成最终冻结验收。第一轮红色是同日且已锁目标先返回 immutableHistory 的预期纠正；第二轮红色是墓碑附言必须清空正文的结构 fixture 纠正，均未据此修改正式实现。恢复补充真实命令生成的非前沿拒绝，以及明确标注的结构测试：较早日期的 pending、池位置内容时钟、锁定当日继续、完整输出 clone 隔离。

recovery-focused-1.log 保留了新增 sequence overflow fixture 被构造器拒绝的红色：无来源根轨迹必须为序号 0。不能以绕过 validator 制造公开命令可达状态；移除该无效 fixture 后另行运行 recovery-focused-2.log。序号上限防溢出分支保留，但无可行大小的有效完整序号链，故不宣称动态命中该分支。目标 priority 溢出仍有有效结构 fixture 与拒绝原子性门禁。没有修改 validator，也没有为同 position 子项发明来源未定义的 tie rule。

冻结后证据写入外置 /workspace/shared/noonmark-carryover-build，不回改本设计以伪造同一版本通过；服务及完整 UI 接通由后续整合任务另验。未执行 Swift oracle，未执行生产或任何真实远端资料操作。
