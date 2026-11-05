# 普通子任务：领域与事务设计

基线 `b5b93453135514b31b9e55e6a8582f470b9563c0`，上游 `43e6eefaae3a04be89cab3e234d470111afd80ef`。仅隔离 development/e2e 合成资料。先在独立副本实施和审查，再冻结后整合；本批不是 UI 接线、正式 Swift 导入或生产存储。

## 源码合同

- `Sources/NoonmarkCore/NoonmarkEngine.swift:1324–1423`：池内 planned 子任务新增、改标题、改难度、删除。当前 definition 原地修改，更新 definition/chain 时钟，不因子任务编辑分叉父标题版本。新增 position=count+1；删除从数组移除，并按现存数组次序重新编号1…n。
- 同文件 `2603–2626`：实际子任务新增要求父 trace pending、日期未锁；position 包含已取消墓碑。源低层入口没有 today，本移植显式路由增加原 UI 同等的 date>=today 防线。新 child 只需有限 own clock；Service 在正常运行时分配全图单调 frontier，不擅加核心不存在的 parent-clock 下限。
- `2628–2685`：今天的 pending 父 trace，pending child→completed / completed→pending；abandon 为低层动作，并未因此添加 UI 菜单。子任务完成不会自动完成父项；开放子任务仍阻止显式完成父项。
- `2687–2763`：今天或未来 pending 父 trace，标题、难度与删除可作用于已完成 child；已取消 child 按 notFound 拒绝。实际删除保留 ID、lineage、position、创建时间，写 cancelledDraft、settledAt 与独立 cancellation ID，并清空 completedAt，不能与 planned 物理删除混用。
- 计划→实际重新分配 ID、保留 lineage；回池只带回开放项并重新分配计划 ID。计划和实际没有独立分类、备注或任意拖放重排命令，本批不发明这些能力。

上述路径以参考仓库实际布局为准；行号是冻结基线的源码定位，不是 Swift 测试通过的证明。领域的子任务进度采用整链 lineage、最高难度权重及历史 floor，本批没有另造计数百分比。

## 路由与事务

核心 `EditPlannedSubtask` 显式绑定 chain/currentDefinition/plannedID；`EditSubtask` 绑定 chain/trace/actualID。类型化命令携带确定的新增身份、取消身份、today 与 now，保证可精确重放。无效编码、空标题、非法字段组合、身份复用、过期 owner、能力或时钟错误均保持完整图不变。规范等价标题 no-op 使用共用 textcanon，保留原字节；真实 CGJ 不能被吞掉。

Service 对外提供 `AddPlannedSubtask`、`SavePlannedSubtaskTitleAcknowledged`、`SetPlannedSubtaskDifficulty`、`RemovePlannedSubtask`，以及对应实际 `AddSubtask`、`SaveSubtaskTitleAcknowledged`、`SetSubtaskDifficulty`、`DeleteSubtask`、`SetSubtaskCompleted`。`AbandonSubtask` 仅低层 API。UI 调用方必须使用同一已存在的 FIFO/原生最终化/发布通道，不可凭 API 存在宣称 UI 接通。

锁内构造私有完整候选，专用审计先验证 prior==before，再对精确命令独立重放，并要求全 after 相等。所有分类历史与其他 supplement 字节冻结；实际子项编辑额外冻结 chains/definitions，planned 编辑冻结 actual subtasks。普通回池创建新 definition；支持子集内禁止 planned 编辑改写历史 definition。最终 normalize 与原严格空 catalog audit 继续执行，不给通用审计添加字段豁免。

标题确认只来自同事务规范化候选，不读取稍后 Snapshot，也不回显请求冒充持久化。no-op仍验证整个磁盘 prefix。提交失败不发布 graph/snapshot/revision，新增 ID/标题无成功确认；一旦存在部分或不确定写入，后续命令拒绝，包括 no-op。保留全部原目录，不修补半事务。

## 明确边界

- 源 continue + snapshot undo 可以恢复 current definition 与旧历史 trace 的别名；core允许相应 planned 原地编辑。当前 Service 既有 history validator 不接受该 undo 的 cancellation 日期事实，故仍在旧 scope 边界拒绝，不能称源不可能出现别名。两个独立探针分别锁定 core 合法与 Service 未支持，不扩张 undo 范围。
- 显式路由先校验 trace/日期/owner，源部分非 add 命令先找 child。因此只读 trace + 不存在 child 的错误优先级可能不同；不宣称所有畸形组合逐项 error parity。合法 unlocked 非 pending 父项的 add 则遵守源 ErrInvalidTransition；locked 优先 ErrImmutableHistory。
- 仍只接空 catalog、普通任务；分类编辑、重复计划、完整undo、SQLite19、同步、AI、真实平台IME、Swift动态oracle未验收。v2会话无静默迁移，回滚不能删除新事实来迎合旧reader。
