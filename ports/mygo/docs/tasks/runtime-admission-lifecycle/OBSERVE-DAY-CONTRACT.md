# observeDay 准入阶段合同

observeDay 使用“UI 准备在先，自然日 worker 入队在后”的顺序。UI 准备并非纯快照：registerEditor、Observe 可能通过 coordinator.pump 排入标题保存，continueIntent 也可能执行原有 lifecycle 行为，包括导航、重入 Close 或 Quit。准备产生的保存必须保留在自然日 Advance 之前。

## 阶段与拒绝

1. 首次 producer 拒绝：本次准备没有执行。调用方仍在原 UI 栈时复位 dayQueued。已接受但未获 UI 执行许可的 producer 只记 UIUndelivered，不从后台修改 UI。
2. producer 获得 UI 许可：按原顺序 register、finalize、Finish、Observe。某一步失败可提前返回并复位 dayQueued；此前已经接受的独立保存继续由自己的 ticket 完成。
3. 准备完成后 writer 拒绝：只复位本次 dayQueued 并返回。该自然日 worker 未接受，其 Advance、publish 和 completion 不执行；拒绝分支不新增 Finish、Replace、清草稿、关闭或 Service 操作。保留 observedDay，避免拒绝自行触发同日期重试。
4. 第二跳拒绝不等于整个准备回合零 native、零 Finish、零排队保存或零写入。原 Finish 可提交已送达的 composition；Observe 和原 lifecycle 也可能合法 canonicalize 或导航。拒绝处理不清除、替换或销毁草稿，也不通过恢复旧字符串回滚这些真实结果。
5. EndRun 后不授予新的 UI 许可；已经开始的 UI 准备回合允许完成，后续新 writer 被拒绝。cleanup 只等已获许可回合归还 activeUI，不等未开始 producer，也不让 writer 等待将来 UI 回调。

## 顺序约束

不能先将自然日 worker 放入同一 lane，再等待 UI 准备信号：这样会把准备产生的保存排到 Advance 后面。若让该 worker 等待后排保存，又会在同一串行 lane 内自锁。因此本接口不引入这种提前占位或 ready-channel 等待。

## 验证边界

- 首次 producer 拒绝：本次准备计数为零，dayQueued 在原 UI 栈复位，无新增自然日 worker。
- 准备后 writer 拒绝：分别记录准备调用与自然日 worker；前者可以非零，后者未接受。拒绝分支不再增加 native/Service 调用，不断言整个回合零写或绑定绝不变化。
- 准备确实提出标题保存：保存先于自然日 worker 入队并执行；即使后者被拒绝，前者仍正常完成，其 receipt 不被取消或改写。
- 准备中途失败：自然日 worker 不投递，已接受的独立保存仍结算。
- EndRun 与已有 UI 许可交错：后续 writer 被拒绝，当前回合结束后 activeUI 归零；未开始 producer 记未消费，不成为清理等待条件。
- 存在 pending lifecycle intent 的场景须保留原 Close/Quit 重入事实，不能把原生命周期副作用算作“拒绝补救”，也不能用无 intent 的测试证明所有准备都无副作用。

该合同只定义终止生命周期 admission，不代表未来 Health 已知停写后仍可继续准备。Health、异常退出草稿恢复和操作系统任意线程行为不属于本切片的验证结论。
