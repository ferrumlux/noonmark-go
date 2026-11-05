# GO-UI-SUBTASK-005：失败新增的原对象失效后不可达

状态：处理中，候选修正待正式整合commit。首次发现：2026-11-05 20:14 UTC。

- 症状：Add失败后，另一窗口ReturnToPool使原actual composer永不挂载；Close/Quit仍阻断，用户无法重试或明确清空。planned definition分叉、完成或跨日也可能令旧输入不可达。
- 引入：本轮未提交UI候选r2；无引入commit，Git author/committer不适用。实际修改为本任务UI工作线程。
- 原红：TestReviewFailedComposerRemainsRecoverableAfterRemoteReturn；独立日志r2-recovery-red.log，本候选evidence/r3-recovery-layout-red.log。新增planned分叉恢复亦先运行红。
- 修正：失败composer不可变surface+target，当前同chain详情提供标明原对象的恢复输入区；保留原Day可达父行。只在明确失败状态呈现，不冒称Swift正常界面原有功能。已失效目标仍由Service拒绝，用户可以复制/明确清空，不自动改绑或丢弃。
- Native证据：真实帧间SetMarkedText，远端complete/return/history/plannedfork后直接读原handle的Text+MarkedText；随后原生明确清空，关闭可释放。TextInputSnapshot.Text本来不含preedit，不能错误当作丢字；原始新增测试的这项断言已按API契约纠正，不改独立原红断言。
- fast gate：TestReviewFailedComposerRemainsRecoverableAfterRemoteReturn。symptom gates：TestSubtaskUIFailedPlannedComposerKeepsOriginalDefinitionReachable、TestSubtaskUIFailedActualComposerRemainsEditableForRecovery。聚合scripts/check。
- 边界：端口防丢稿增强，不增加普通草稿autosave或跨崩溃恢复。错误文案为“添加尚未确认”，after-persist不得断言没有持久化。
- 教训：关闭安全屏障必须配可达解除路径，不能将防丢稿变成不可退出的死锁。最终状态待正式修正commit，回滚仅隔离开发UI。
