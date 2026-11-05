# GO-UI-SUBTASK-002：旧难度菜单回调跨会话重挂

状态：处理中，候选修正待正式整合 commit。首次发现：2026-11-05 20:05 UTC，独立审查真实 MyGo Tester 反例。

- 症状：打开子项难度菜单后保存 native callback，clean Close、Service Close/Open、同 app 重新 attach，旧 callback 可把新会话的子项难度从简单改为困难。未发布，只有隔离合成 fixture。
- 引入位置：本任务未提交的 runtime_subtask.go，菜单key只含surface owner与navigation count。没有引入commit；Git author/committer不适用；实际修改为本轮UI工作线程。
- 机制：window显示ID和navigation可从初始值重新使用，MyGo host据相同key复用了菜单状态。对象ID绑定和旧title registration拒绝不能替代菜单host生命周期身份。普通navigation、菜单取消重开、删除后的反例原本已绿，未覆盖的确是整个Service重挂。
- 证据：独立原反例 TestReviewSubtaskOldMenuAcrossReattachedSession；本副本原样复现于外置 evidence/independent-reproduction-red.log，difficulty 1→3。无需推测或改断言。
- 根因修正：遵循既有父任务行菜单，将不可复用的 *titleWindow instance 加入菜单key，同时保留owner和navigation。旧native callback不能找到新host。
- fast gate：TestReviewSubtaskMenuCancelledAndNavigated；symptom gate：TestReviewSubtaskOldMenuAcrossReattachedSession。均纳入 scripts/check。
- 发布与回滚：正式commit确定及最终全量验证后才关闭案例；撤回UI层，不回滚用户事实或Service。结论仅覆盖本地native headless，不声明OS/Swift oracle。
- 教训：对象身份、表面身份、navigation代际与窗口实例生命周期是不同边界，任何一种都不能替代其余边界。
