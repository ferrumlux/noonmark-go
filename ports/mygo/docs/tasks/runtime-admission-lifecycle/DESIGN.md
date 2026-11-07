# M1/M2：运行期准入生命周期与 utility 关闭前保留草稿

2026-11-07。公开源码基线为 c441d3b331dd3b3358d171f3c2d7e420bb9c2099，tree c3658b612621a45705faf12c3f611e070186ddee；八个来源文件及其 SHA256 见 SOURCES.json。本切片只覆盖运行期准入与 utility 关闭草稿保全，不包含 Health、slider、容量/FIFO 策略、背压、恢复引擎、导出或 RecoveryClose，也不解除生产准入门禁。

## 决策

选定两层短锁准入，**采用固定实现**：
1. SerialLane新增TryEnqueue，与StopAfterDrain用其现有同一mutex。
2. 新runtimeAdmission以一个mutex串行化新的producer、实际enqueue与终止phase切换。producer与已接受worker completion分开。统一锁序admission.mu → lane.mu，闭包执行、Service、RunOnMain、用户回调、cancel函数及Done等待均在锁外。
3. Service.Close仍唯一由main在writer和dispatcher drain后执行；不在admission内关闭Service。
4. utility存在closeWindow时完全不预清draft，不缓存“稍后清空”闭包；获准销毁的app无需清。Close被拒绝，原binding/文字自然保留。无需等Health实现，也没有“清后恢复”竞态。

## 1. lane基础API

`TryEnqueue(f func()) bool`：锁住lane.mu；若stopped返回false且不保存/执行f；否则append并Signal，返回true。true就是队列取得所有权，worker按现有顺序恰好调用一次。StopAfterDrain在同锁置stopped并Signal，幂等，Done仍表示已接纳闭包全部返回。其线性化点分别是append与stopped置位。

旧Enqueue可保留为`if !TryEnqueue(f) { panic(...) }`供旧测试/明确断言用途；所有生产runtime调用迁往Try接口。无容量检查，**仍是无界lane，不称bounded**。本切片不改顺序、不丢队列、不引入优先级或合并。

## 2. runtimeAdmission具体接口

new runtime_admission.go拥有writer、dispatcher、受锁phase和未终结投递记录；不拥有Service或native控件。接口职责：

- `TrySubmitWriter(work func(postCompletion func(func()) bool)) bool`
- `TryPostProducer(deliver func()) bool`
- `EndRun()`：main在App.Run返回后立刻调用，幂等；关producer并停止writer准入。
- `WaitActiveUI()`：EndRun 后仅由非 UI 清理调用，等待已经获准执行的 UI 回合结束，不等待尚未开始的回调。
- `FinishWriterDrain()`：仅main在writer.Done之后调用；关闭completion准入并StopAfterDrain dispatcher。
- `FinishDispatchDrain()`：仅main在dispatcher.Done之后调用；归类所有未消费结果并返回生命周期统计/未消费状态，不触UI。

phase为Running → WriterDraining → DispatchDraining → Closed。producer只有Running；completion只允许Running/WriterDraining，且必须来自真实已接受work的私有postCompletion closure。调用方不能自行构造work token或用producer通道冒充结果。所有Try在admission.mu内检查phase并调用对应lane.TryEnqueue，不得先释放锁再enqueue。

TrySubmitWriter接受时先注册唯一work记录，再enqueue包装闭包；若lane拒绝则撤销该记录并返回false。wrapper只在writer线程调用work(postCompletion)，返回后在defer中记录workerDone。即使业务error也返回正常完成，不改变结果。该callback不得另起goroutine后逃逸postCompletion；work结束后再用它返回false并登记协议错误，不投递。调用者必须在返回前发完结果/Health/图包。

TryPostProducer接受时注册delivery记录并dispatcher.TryEnqueue；dispatcher闭包锁外调用RunOnMain(uiWrapper)。uiWrapper实际开始时才申请UI执行许可：只有Running且该delivery未终结，才标RunningUI并在锁外执行deliver；finally标Consumed。若phase已非Running，标UIUndelivered，不执行deliver。该wrapper可被框架丢弃，所以dispatcher闭包返回不能标Consumed。

worker的postCompletion类似，但独立于producerClosed：即使WriterDraining仍允许进入dispatcher并记录结果投递。**它不保证loop结束后会执行UI**。uiWrapper同样只在Running时进入；结束后标UIUndelivered，不调native，不排新writer。结果对象由worker闭包捕获并归delivery记录所有直到终结汇总，不改原Err/receipt。RunOnMain若直接丢弃closure，FinishDispatchDrain仍能将记录判UIUndelivered。

每个work可发多个completion包（现有adapter阶段publish与最终complete），每包有独立delivery ID；workerDone仅在所有包的post尝试之后。不能用“有一个UI回调执行”替代其他包消费。live阶段的dispatcher若意外拒绝completion，这是协议故障：记录DeliveryRejected，**不能称操作已成功展示或flight已释放**；禁止直接在writer调用UI补救，验收必须失败。正常运行中dispatcher仅由FinishWriterDrain停止，故此分支应不可达，故障测试需证明不被吞。

为减少接口扩散，coordinator.Config替换Lane/Post用法为：`TrySubmit func(func(post func(func()) bool)) bool`与`TryPostProducer func(func()) bool`。运行期coordinator worker使用其获授post发complete；timer/registry retry用producer。runtime hub的submit wrapper在单writer闭包内临时安装writer-only的completion sink供titleServiceAdapter.publishFromLane使用，返回前清除；只在该writer同步调用链访问，UI不得读。category/普通command/observeDay直接用闭包参数。native business亦收到其work的sink。不得留`h.post`含糊地同时代表两类投递。

## 3. 停止顺序与异常Run返回

App.Run返回立即EndRun，先于日志/报告或业务等待：
1. admission锁内Running→WriterDraining，writer.StopAfterDrain（同锁序），拒绝所有后来producer/新writer。锁外调用business.StopAdmission及其他已有cancel标志，不持admission锁拿business.mu或coordinator回调。
2. main等待writer.Done（非UI回调，不持锁）。已接受writer完整执行或以后被Health gate拒绝，其completion sink继续可投递。已接受但尚未在UI执行的producer不会再产生writer，因为UI wrapper发现phase已结束。
3. writer.Done后执行既有business.PreClose。接着FinishWriterDrain在锁内关completion、phase→DispatchDraining、dispatcher.StopAfterDrain；此后无合法worker再能提交completion。
4. main等待dispatcher.Done；FinishDispatchDrain把未执行/已丢弃UI闭包判UIUndelivered，汇总workerDone/consumed/undelivered/rejected数量；再按原main唯一路径Service.Close。PreClose失败仍须执行两个drain和Close，保留错误链。
5. Close失败照原runtimeExitStatus报告，不重开/复活，也不重复Close。

Run正常关闭和启动失败均使用此顺序。启动时category enable失败尚无UI，但若已创建admission也走同一drain，不能旁路Stop使Try接口仍认为Running。无hub/headless的独立同步Close路线保持不变。

生产UI安全前提：uiWrapper和native回调由MyGo UI线程串行执行，App.Run在该loop退出后才返回。EndRun后不再新授UI许可。为不把此假设当万能原子性证明，admission计数activeUI；主线程cleanup若观察activeUI非零必须等待其已获许可回合结束（锁外条件等待，不在UI callback等待）。测试并发调度器可模拟先获得UI许可再EndRun，该回合属已开始工作，完成后main才进入drain/Close；禁止等待一个尚未开始的UI回调。如果真实平台Run返回与仍运行native回调不符合此约束，记录平台阻塞，不宣称本小切片保证任意框架线程行为。

异常Run返回可能导致已接受结果UI未消费。worker执行完成≠UI已确认；计数分列，不能伪造live窗口pending清零。此时不从main/worker调用coordinator/native清理，不做强制Finish/Replace。未消费结果保留到本次报告汇总，内存草稿不保证异常退出后恢复；这是缺失能力声明，不是“丢completion安全”。正常live路径任何未消费completion都不算验收成功。

## 4. ticket/拒绝结算（具体归属见 PRODUCERS.md，observeDay 阶段边界见 OBSERVE-DAY-CONTRACT.md）

区分三种对象：现有UI业务ticket（commands/flight等）、admission work记录、delivery记录。后两者仅线程安全生命周期元数据和原结果引用，不拷贝native正文或做磁盘日志。

- 首次UI TryPostProducer拒绝：调用方仍在原UI调用栈，立即调用自己的`notAdmitted`分支结算已设置的queued/pending/commands；不清文本、不调用native、不伪造保存失败已落盘。可改成Try成功后再设UI标记的，必须保证post永不inline；统一推荐先登记再Try，失败显式撤销调度标记。
- 已accepted producer到UI、第二跳TrySubmitWriter拒绝：如果UI仍持许可则原UI栈执行同一notAdmitted完成，恰好一次；如果producer从未获UI许可而loop已结束，只记录UIUndelivered，不在后台结算UI字段。
- coordinator已BeginPersistence、flight已创建但TrySubmit=false：在当前UI安全栈调用真实`complete(e, request, PersistenceResult{Err: ErrNotAdmitted})`，不设置VerifiedText；若terminal则记录未消费而不native读取。不得简单`flight=nil`或回滚session。适配ErrNotAdmitted只表示此请求未调用Service，不推导同命令先前Advance是否写入（此分支发生于整个worker未接受）。
- 已接受worker中的Service结果无论成功失败都走completion；已入flight不能因健康/终止而静默drop。live UI complete先结算原ticket，再检查新owner/generation，保留新草稿；重复回调以ticket唯一终结位防双减。
- timer redraw没有业务ticket，TryPost拒绝可结束该timer；coordinator的job/intent或clearQueued/registration queued不是纯redraw，live拒绝需处理自己的调度标记。后台timer拒绝只记生命周期，不后台改UI字段；terminal后不需伪造该job已保存。
- EndRun之后恰好一次“worker终结”与“UI未消费”可成立，不能同时声称UI flight已释放。记录不能无限积累到另一个会话：consumed可立即从delivery表移除并计数，undelivered在本次drain报告后释放；量随当前未完成工作，**不宣称固定内存上界**。

## 5. M2 exact utility行为

将dismissUtility顺序规定为：

1. 若closeWindow != nil：仅调用closeWindow后return；无论clearDraft参数真假，不写s.draft，不Finish/Replace，不建立清空授权。
2. 若closeWindow == nil：保留原无窗口fallback语义，在同UI回合按clearDraft清s.draft再设置page=fallback。此为无native窗口的显式导航，不是模拟成功Close；未来若给该路线接hub/native owner需重新设计，不暗继承批准。
3. 不在返回后清空：Window.Close可能veto或异步。获准关闭的窗口由框架销毁，app无需人为清。不会为了保留字符串而还原旧值覆盖迟到输入。

M2 本身不依赖 M1 或 Health；它修正既有关闭路径的确定性缺陷。只新增这一个顺序修复，不扩展普通utility取消行为。既有窗口OnClose/beforeClose/commands/native门仍裁定是否关闭；后续Health gate可在同位置新增拒绝，draft自然不丢。

## 6. 有限文件边界

新增runtime_admission.go及针对性测试；修改internal/titlecoordinator/lane.go（TryEnqueue）、coordinator.go（Config投递分类/拒绝complete/timer）、runtime_title.go（hub投递与每入口拒绝）、runtime_category.go（两跳/900ms/输入post）、runtime_subtask.go、runtime_title_disposal.go（producer结果）、native_business_probe.go（TrySubmit/已接受sink）、main.go（admission装配/EndRun顺序/M2）。如果实际h.post引用还有文件，由静态清单逐个列明，不能以全仓替换字符串迁移。

不改Service、domain、codec、MyGo、持久化、typed outcome；不增加RecoveryClose。成功业务语义与源UI视觉不改。原测试 Lane/Post 替身显式迁移并保留原断言；拒绝矩阵用可控拒绝测试独立验证。有限测试不替代完整门禁、各平台编译或原生窗口验证。

## 7. observeDay 的阶段边界

observeDay 必须保持 UI 准备产生的保存先入队、自然日 worker 后入队。首次 producer 拒绝时本次准备没有运行；原准备完成后 writer 拒绝时，仅保证该自然日 worker 未接受，拒绝分支不新增 native 或 Service 操作。准备可能已 Finish、排入独立保存或触发合法 lifecycle intent，不回滚，也不推导整个回合零写。完整合同及验证边界见 OBSERVE-DAY-CONTRACT.md。
