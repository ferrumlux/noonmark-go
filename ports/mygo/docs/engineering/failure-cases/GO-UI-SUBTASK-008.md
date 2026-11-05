# GO-UI-SUBTASK-008：旧 planned owner 草稿缺少恢复标识

状态：处理中，候选修正待正式整合commit。首次发现：2026-11-05 20:26 UTC。

- 症状：planned标题dirty时父标题合法分叉，新definition保留同child ID，UI保留旧owner/native草稿并锁定，但仍像当前已保存子项显示，没有原对象草稿标识。
- 引入：本轮未提交childRows只以child ID判定是否present；无引入commit，Git author/committer不适用，实际修改为UI工作线程。
- 原红：独立TestReviewForkedPlannedDirtyEditorIsLabeledRecovery，r3-planned-draft-label-red.log；target/文本/锁定安全断言均已通过，仅缺少明确标识。
- 修正：完整不可变target不同，或被锁定的owner仍dirty/error，按原editor行显式渲染为retained恢复资料，不把当前definition行冒充旧native草稿。行内说明“原对象的未保存草稿”，禁止编辑/操作新对象。
- fast gate：TestReviewForkedPlannedDirtyEditorIsLabeledRecovery；symptom gate：TestSubtaskUIPlannedDefinitionFenceRetainsDraft。聚合scripts/check。
- 教训：稳定child ID不能替代owner的完整版本身份。保住文本还不够，用户必须知道其未确认状态和原对象归属。正式整合commit后再关闭。
