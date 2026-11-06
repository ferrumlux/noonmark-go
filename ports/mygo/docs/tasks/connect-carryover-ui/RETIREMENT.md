# 自动标题退休独立所有权（R6／R7）

最终实现与验证以末尾「精确同步退休」及「R7 成功退休后的事实协调」为准；已在 `961e688` 整合。下面保留原设计和修订依据，低优先 navigation intent 方案没有实施。

R5新强原红：Defer writer已完成、publication待UI执行；用户已点击切pool，titleTransition写入最新用户target；稍后的publication清理又用queueTitleTransition覆盖该target，最后仍day。R5仅验证反向顺序不足，原独立断言保持不变。当前不改R5冻结稿。

## 不可混同的所有者

用户navigation拥有s.titleTarget、h.targets闭包、window.navigation；自动清理不是用户意图，不得写这三者，也不得清除close/quit。自动清理拥有独立request，捕获窗口对象、navigation值、完整原title editor指针及registration身份。只允许清理该集合，不以回调时当前集合替换它。

## 准入与执行

1. publication只提出低优先清理request。已有用户target、closing/quitting则不提出；重复同集合请求合并，不影响用户意图。
2. UI队列执行前再次验证window、navigation、用户target为空、close/quit、完整editor集合身份一致。失效则丢弃，不重排，不用新editor重新解释旧请求。
3. 保留普通composer真实Finish，稳定挂载恢复owner不重复Finish；清理不改变composer挂载条件。失败停止，保留对象及真实错误，不能猜空。
4. coordinator提供低优先处置准入，只有无全局quit且该window无任何pending intent时接受；禁止通过既有RequestNavigation清空其他intent。仍使用既有finalizeAll/proof/clean/dispose机制，不增加同步Service调用。真实用户navigation随后可以取代该低优先处置，关闭/退出也按原优先级处理。
5. 专属处置完成只清理本request捕获、且仍匹配的editor。不能沿普通Navigate回调无条件清空整个titleEditor/subtaskEditors；没有page/date/selection闭包。若同期注册改变，coordinator原epoch proof fail-closed，UI身份检查也不得释放新editor。
6. 新用户navigation不受自动清理影响，旧request不吞用户目标或复活旧闭包；失效清理请求可等待下次真实publication重新评估，不自动重试磁盘。disposal request与source guard独立，绝不在此增减source引用。

## 门禁

- 保留R5全部原红；新增publication在前、用户nav已请求、cleanup后执行的真实独立原红。
- 两种排列：cleanup先→用户后，用户先→cleanup后；用户往返形成新title session时旧清理不能移除它。
- close/quit先在coordinator持有intent时低优先清理拒绝；另一窗口intent不应阻止本窗无关clean处置。
- 同步Changed回调重入注册/用户navigation使proof失效，不能处置新对象；不修改既有coordinator原断言。
- 普通空composer历史化四Continue仍需原真实Finish；稳定恢复重新focus租约保持；真正navigation全部Finish。
- 新freeze重新完整scripts/check、六compile、CLI，R5绿不冒充R6。

## 优先方案：精确同步退休，不再建立低优先navigation intent

以上第4步改为受控 `TryRetireEditors(window, []RetirementOwner{Registration, Identity})`。该调用只运行UI线程，无IO、无异步保存、不建立任何intent、不调用Navigate。app独立队列request仍按前述身份/fence负责作废。

- 准入：无global quit、无本窗口任何user navigation/close intent，未stopped；捕获列表每项精确reg+identity/generation属于此window，且为locked clean（不dirty/composing/inflight/awaiting/error）。不注册新editor，不Observe，空列表无操作。
- 捕获coordinator registry epoch，逐项native Finish前后检查epoch、每项identity和无用户intent。任何native回调重入改变注册/用户intent即停止，不取消新intent。
- Finish失败/composing返回ErrNative；不走通用fail（它会清intent），不触发Service/重试。应用保留原对象并显示原失败消息。
- 对成功fresh snapshot用原Session.RequestLifecycleFlush更新实际证明；若late text导致dirty，保留session/native，返回未退休。因为仅locked对象，不安排保存，也不清空草稿。通过Changed更新UI状态时发生重入，必须再次验证原epoch/identity/intent，不能继续dispose。没有DraftReplacement写入。
- 全部fresh snapshot flushed且集合未变，使用原Session.Dispose及registry removal，内部无用户callback，逐项只删除指定原reg；不扩展到后来注册者。返回明确退休结果，app只删除capture仍匹配的editor，保留任何新对象。失败无持久cleanup槽，不阻碍下一次用户navigation。
- 其它窗口intent不影响本窗，仅注册epoch变化会保守拒绝本轮，留待下次真实publication重新评估。自动清理不负责推进持久化或复活导航。

这里保留普通composer Finish的位置必须在app复核用户target为空后、coordinator准入前；如果该Finish引发回调，重新检查全部app身份/fence后才调用退休API。稳定恢复owner继续保留其已重新focus的lease。应用层与coordinator都不能以缓存clean布尔代替本次真正native证明。

## R7 成功退休后的事实协调

独立合同红（合法Changed hook重入app.titleTransition，并非已证明的OS路径）揭示：coordinator已成功退休，app后置request失效检查却忽略成功，留下registered=true但不存在的reg。成功副作用不能被presentation fence抹掉。

成功返回retired=true且err=nil后，必须协调精确捕获的pointer+registration+identity仍匹配的原owner，标记其registered=false并仅从仍指向该原owner的app槽移除。此阶段不再要求request/navigation仍current，也不执行任何旧页面闭包，不改titleTarget/targets/navigation，不清替换的新owner。retired=false或err不移除原owner，无异步重试。所有准入前/Finish后fence继续存在；只有已发生精确退休的事实收尾独立于展示生命周期。

实施只采用精确同步退休，低优先navigation-intent方案未实施。R7的事实收尾不更改用户navigation，仅协调已退休的精确原owner。
