# GO-RUNTIME-012：子任务新增错误类别与父能力顺序

日期：2026-11-06（新加坡）。此缺陷在尚未发布的独立子任务候选中被审查发现，没有进入旧远端提交；不能把未提交候选冒称历史生产缺陷。

## 原始反例与原因

独立测试 `TestReviewAddCompletedParentError` 先通过正常 `MarkCompleted` 完成无子项父 trace，再调用合法 `EditSubtask(add)`。源 Engine:2611先检查未锁日，2612–2614对非pending父项抛 invalidTransition。候选把所有非pending父项与历史日期合并为 immutableHistory，原测试实际为红。

修复拆开历史日期、锁日与父status守卫：历史/锁定仍immutableHistory；unlocked非pending时，add返回invalidTransition，其余编辑保持immutableHistory。没有放宽任何实际可写能力，没有修改图或存储字段，没有通过只更改测试期望掩盖差异。

## 永久门禁

原审查反例原样保留行为并加入AGPL标头/gofmt；`TestDaySubtaskDatesOwnersAndClocks`覆盖各命令与日期/锁日/父状态/owner/反向时钟矩阵。领域每次拒绝必须保留完整graph。Service覆盖所有命令四阶段写入失败、并发确认、完整重开及no-op prefix检查。

同轮独立审查找到源undo产生历史definition别名的合法路径。两个独立绿探针 `TestReviewUndoContinuationAliasCoreEdit` 与 `TestReviewUndoContinuationAliasServiceScope` 分别证明core合法编辑、Service在既有unsupported cancellation fact处拒绝；未证明新增历史保护在当前支持子集额外误拒。报告保留边界，未移除fence或扩张undo功能。准备的Swift测试未运行，不称oracle通过。

完整门禁入口 scripts/check；跨平台编译不替代实机。原始红与独立绿日志外置保存；最终整合提交和完整执行结果见 [验收记录](../../tasks/connect-subtask-core/VERIFY.md)。
