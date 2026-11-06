# 通用新轨迹 batch 证明模型

状态：组长已审核有界模型，已实施完整 canonical 命令见证；资格／origin proof 原型已移除。

## 实证迫使重设计

旧入口接受剥掉关联的延续。首轮资格探针分别重复使用同一个 before pool membership，并未约束新 trace 的 current definition。独立反例还证明 empty-before 新 chain 可绕过仅检查 existing chain 的分支。这三项是同一 GO-RUNTIME-014 根因，不以字段黑名单逐个打补丁。

## 拟支持的完整模型

仅在 generic capture 出现新 trace 时启用批次证明，输入仍为 exact before 与 after。生成一个可审计的候选命令序列，只允许实际 core CreatePoolTask、ScheduleFromPool、MarkCompleted／UndoCompleted、SettleTaskDays。将全部命令重放于私有 before kernel，最后以原 supplement 编码并比较完整 canonical after，没有字段剥除、终态字段豁免或任意复制新 chain。既有冻结与空 catalog 验证继续运行。

1. 新 chain 只能由 CreatePoolTask 生成：新 identity、唯一 sequence=1 current definition、原始 title/description、至多一个未删除 initial note、createdAt。命令执行后所有 chain/definition 字段最终必须与 after 相同。新 chain 的任意历史版本、删除备注或专用编辑不能靠导入掩盖。
2. 新 trace 全部由同一个重放 kernel 顺序 ScheduleFromPool 生成。目标 date、trace/day identity、planned child identity 来自 after；planned child 必须精确映射当前 definition。排序按真实 createdAt，同刻按候选事实的插入顺序，core 亲自验证 priority。每次真实命令都重新检查池资格，因此一 chain 无法重复消费同一资格。新链与旧链没有例外。
3. 用已有／新增 trace 的 final status、CompletedAt、ContentUpdatedAt 构造普通完成／撤销命令。无变化不添加命令。完成→撤销等终态信息不足的组合必须明确列出可证明条件，不能猜测后忽略不匹配。只要给出一条真实合法命令序列且完整终态相同，便足以证明 structural admissibility；不声称恢复真实操作日志。
4. 新 day lock 按 clock/date 重放 SettleTaskDays，真实 core 自行结算 trace 和 children；不能手写 unfinished 或 settledAt。锁定必须保留 before 的既有锁。整图最终比较验证全部新／旧事实的状态、时钟、内容与 child 数量。
5. 源 API 的 Today 不存于 graph。审计只用 trace date 作为排程／完成的结构合法见证；真实 Service 执行时仍绑定实际 Today。这项证明不是真实用户意图或日历授权的替代。

## 明确边界与兼容风险

无新 trace 的通用调用保留既有审计规则，此设计不是声称补齐全部通用 mutation 的命令溯源。Carryover 专用完整 replay 不改变。

混合 title/version、manual progress、subtask edit、return、defer/continue 的 generic batch 不在本模型。这些操作已经有各自专用 exact replay 审计；不能以 generic 状态组合绕开它们。若现有实际调用或症状测试证明需要这些混合批次，先报告具体组合与源语义，再扩充显式命令模型，不自动删断言、弱化证据或冒充兼容。

新 chain 自带多条 notes、planned-subtask 编辑历史，或同一 trace 多次完成／撤销导致无法重建有限明确见证时，必须 fail-closed 并记录具体组合。现有公开 Service 的每次 Create、Schedule、Complete、Settle 都有独立事务；另有批次正例证明 empty-before 三条 Create+Schedule、其中一条 Complete、两个当日 trace Settle、未来 trace Pending，以及 existing-pool 多链 Schedule+Settle 都不得误拒。

## 验证

保留所有原始红断言及独立红证据。新增明确正例／负例：新链和旧池多链批次、planned child materialization、Return 后合法 Schedule、Complete+Undo 的可证明边界、不同日期与时刻的 settlement。重设计通过集中及独立审查后才重新生成 freeze manifest，再跑 race/vet/aggregate。旧 freeze manifest 已明确作废。

## 审核后实施约束

组长已接受有界方向：不保留 eligibility-only fallback。实现固定一次命令见证，不做无界搜索。相同时刻依赖为 Create、Schedule（保留实际插入顺序，core 验证 priority）、Complete/Undo 的必要二步、Settle。Pending 的完成→撤销见证使用最终 content clock；Unfinished 的完成→撤销见证使用保留的 chain clock，并由最终全图相等验证。任何无法由该固定见证重现的 after 都 fail-closed。

明确当前不支持：新 chain 在同一 generic batch 带 planned-subtask 编辑（单独 CreatePoolTask 无该参数）、多条 initial notes 或历史 definition；新 schedule 同批完成 planned children 后再完成父项（child edit 属专用审计）；无法用最终时钟重建的复杂反复修改顺序。没有把这些组合说成唯一真实历史，也不证明当时用户授权。真实 Service 单命令及列明的普通组合必须保持。

## 整合审查补充

固定 Instant／依赖排序不是任意合法 core 命令序列的完备搜索。跨链非单调时钟可使实体插入顺序与固定见证不同，导致 fail-closed；不能据多链正例宣称接受所有合法批次。正常 Service 单命令事务使用统一单调 frontier，其实际路径已经回归。
