# 显式producer和ticket所有权清单

固定 c441d3b 公开基线生产行号（见 SOURCES.json）；P=TryPostProducer，W=TrySubmitWriter，C=获授postCompletion。全部由实际同锁Try决定接受，不以事先检查bool代替。

|来源|分类/业务所有者|live拒绝的确切处理|
|---|---|---|
|runtime_title.command 629→630→644|P→W→C；w.commands/inputPending，外层actionPending/carryover/composer回调|首/第二跳失败均走统一finishNotAdmitted，commands减1，inputPending删原key，外层completion调用一次；不走成功clear|
|runtime_title.observeDay 530→546→556|P→W→C；dayQueued/observedDay|复位dayQueued；保留已观察日期，拒绝不自排重试。首P拒绝本次准备0调用；原UI准备后的W拒绝只保证该自然日worker未接受，拒绝分支不新增native/Service。原准备可能已Finish、排保存或触发原lifecycle intent，不回滚、不称整回合零写|
|runtime_category.submitCategory 177→178→188|P→W→C；w.commands和原editor.pending/generation|结算旧editor.pending和commands，保留native draft、creating及route；再检是否当前editor以决定显示错误，不先丢旧ticket|
|coordinator.pump 296→313|W→C；原SaveRequest/e.flight/session.InFlight|UI调用complete原request+ErrNotAdmitted，保留draft，无VerifiedText；已接受worker不走此分支|
|native_business_probe 175→306|W→C；b.started/ack/cancel/报告|TrySubmit拒绝时b.finish(准入拒绝error)解除run等待，不假装UI ack；accepted completion未消费时仍靠StopAdmission取消run，报告UIReadViewAcknowledged=false|
|runtime_title.publishFromLane 446/460|C；当前accepted work，图/后续Health包|记录该包DeliveryRejected/Undelivered，禁止worker同步publish；不能把最终complete已消费当此包成功|
|coordinator timer 214|P；entry.timer/job/session|后台拒绝不动entry/native；terminal记录后结束。已进UI后token/stopped检查仍在；不能把job当saved|
|coordinator.registryChanged 394|P；intent.recheckQueued|UI Try拒绝复位recheckQueued并终止该intent/错误回调，不删除draft或生成新保存；terminal未获UI许可则记录未消费|
|category成功提示900ms timer 227|P；仅redraw，无保存ticket|后台拒绝直接结束，不设置saved或调用native；不能绕过dispatcher停止|
|category OnSubmit/Create 339/365|P；尚未submit，无pending ticket|拒绝无Service/无新增pending，文本保留；可记未准入提示|
|category Cancel 347|P；编辑器creating/native|拒绝不得清空/Finish；仍保留当前编辑状态|
|titleTransition 343|P；targets[target]/s.titleTarget/navigation|UI拒绝删除仅自己target；若s.titleTarget仍相等则清该调度标记，不执行fn，不清editor；terminal未消费不后台碰map|
|liveTitle注册378 / subtask注册465|P；e.queued|UI拒绝将**原e**.queued复位，仅本次捕获实例；不换native绑定。不得将新owner queued清掉|
|liveTitle Observe387 / subtask Observe474|P；seenText/seenCompose优化标记|UI拒绝保留原native；不要推进seen标记成已观察，恢复上一个seen或使用显式needsObserve，不覆盖用户文字。后续正常可再Observe|
|liveTitle blur392 / subtask Enter454/blur479|P；捕获reg/id|拒绝不调用EndEditing/Finish，草稿留原处，不虚构已flush|
|runtime_subtask.clearQueued 529|P；composer.clearQueued/hasAcknowledged|UI拒绝clearQueued=false，ack事实保留，绝不清native；terminal仅记录|
|runtime_subtask.submit535|P；submit尚未进入，无pending|拒绝无提交，无native操作|
|runtime_title_disposal 58|P；s.titleDisposal==r|UI拒绝只在仍同r时清调度指针，不retire，不改titleEditor/subtaskEditors/native|
|newTitleDispatch 676|改为admission底层dispatcher.TryEnqueue|不再暴露无条件Enqueue的h.post公共入口|

## 接受后、UI未执行的producer

被dispatcher接受但RunOnMain未执行的P不意味着其业务已开始。EndRun后wrapper拒绝进入UI时登记UIUndelivered，保留原UI对象直到进程生命周期收尾，不从后台调用上表live拒绝分支。报告区分：producerAccepted/producerConsumed/producerUndelivered，writerAccepted/workerDone，以及completionConsumed/Undelivered/Rejected。

source kind可用固定枚举作统计标签，无用户任务文字。记录只拥有投递闭包与原结果对象，不拥有操作native的权限。禁止复制草稿到日志、导出或远端。

## native business锁序

b.run当前在b.mu内Enqueue。迁移后可在b.mu内调用admission.TrySubmit（只短锁，不等待）；全局顺序为b.mu→admission.mu→lane.mu。EndRun只持admission锁变phase/stop，释放后才调用b.StopAdmission；绝不admission→b.mu。work运行在锁外。b.finish/ack不得被admission调用时持锁。现有PreClose报告失败不得短路dispatcher drain或Service.Close。

## 完成消息的反向生产

C在Running期间可以正常执行原UI completion，其后若需刷新/新job，必须另走P/W准入；C不是无限授权新writer的入口。EndRun后不授UI许可，所以不产生反向工作。一个accepted work所有C须在work返回前提交；无逃逸goroutine。live C投递失败是明确协议失败，不允许仅返回false后把原operation当成功，也不在worker直接减UI计数。

## 非本切片范围

runtime_ui c.After(time.Minute)为MyGo框架重绘，重绘后的observeDay走以上受控入口；不改框架timer。原SerialLane未限制长度，未提供全局输入FIFO/拖动容量合同。OS任意关机/强杀不由本表证明安全。正常Close可veto、异常Run返回可丢UI投递，均不等于RecoveryClose。
