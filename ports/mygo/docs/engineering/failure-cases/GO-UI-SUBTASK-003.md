# GO-UI-SUBTASK-003：旧新增确认吞掉新输入的Return

状态：处理中，候选修正待正式整合 commit。首次发现：2026-11-05 20:06 UTC，独立审查真实 native 帧间反例。

- 症状：composer提交A；A落盘前native开始B的marked text；A确认因composition而保留待清理ack。OS在帧间提交B后用户一次Return，只清掉旧ack，没有添加B，需再按一次Return。
- 引入位置：本任务未提交的 runtime_subtask.go/submitSubtaskComposer，hasAcknowledged分支无条件return。没有引入commit；Git author/committer不适用；实际修改为本轮UI工作线程。
- 机制：清理旧提交文本与处理当前提交手势错误地互斥。旧ack处理取得fresh native proof后若发现不同的已提交新文本，应继续当前手势，而非当作已经处理。
- 证据：TestReviewSubtaskComposerReturnAfterMarkedLateAck，原样在本副本复现于 evidence/independent-reproduction-red.log：child只有A，draft=B，ack=false。独立日志composer-late-ack.log亦已留存。不是Service丢写或700ms问题。
- 根因修正：清理旧ack后，只有仍未获得native证明时才返回；已确认不同的新文本继续执行同一次Return的真实Add。若旧提交文本被安全清空，后续空文本守卫自然阻止重复新增。
- fast gate：TestSubtaskUIComposerFailureAndNewerNativeDraft；symptom gate：TestReviewSubtaskComposerReturnAfterMarkedLateAck，scripts/check强制执行。
- 发布与回滚：正式修正commit和最终全量证据确认前保持处理中。只撤回UI批次，不覆盖领域snapshot。
- 教训：native输入、旧异步确认和当前用户手势必须分别判定；不能用完成清理作为吞掉当前意图的理由。
