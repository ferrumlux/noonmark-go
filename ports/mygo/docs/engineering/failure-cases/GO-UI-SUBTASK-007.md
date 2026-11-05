# GO-UI-SUBTASK-007：被删子项草稿误计入持久子任务数量

状态：处理中，候选修正待正式整合commit。首次发现：2026-11-05 20:19 UTC。

- 症状：唯一child在另一窗口删除后，原生dirty草稿正确保留，但胶囊仍显示“1 子任务”，把恢复资料混入确认事实。
- 引入：本轮未提交runtime_day.go使用childRows长度计数；无引入commit，Git author/committer不适用，实际修改为UI工作线程。
- 原红：独立TestReviewRetainedDeletedDraftDoesNotCountAsSavedChild，r2-ghost-count-red.log。原草稿保留断言与计数断言原样保留。
- 修正：childRows仍可含恢复ghost；胶囊计数只用actualSubtaskRows持久可见记录。仅有ghost时显示“未保存草稿”而不是子项数，并在其行内标明原对象草稿。领域子项、取消墓碑及日统计不改变。
- fast与symptom gate：TestReviewRetainedDeletedDraftDoesNotCountAsSavedChild；TestSubtaskUITwoWindowsDuplicateAndRemoteDeleteDraft。聚合scripts/check。
- 教训：输入恢复集合不等于领域presentable事实集合。任何数量都必须明确来自已确认快照；这同样不构成chain lineage加权进度实现。正式commit后更新状态。

补充测试审查：初版HasText使用substring，可能把“展开子任务 <以1开头的UUID>”误认为“子任务 1”。已保存诊断r3-count-diagnostic.log；独立审查者将计数断言改为逐项exact text，仍在r2旧实现证实原症状红，保留draft与持久child0断言。没有以改断言掩盖计数问题。
