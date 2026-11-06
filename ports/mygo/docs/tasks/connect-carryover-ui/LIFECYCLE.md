# Day carryover 输入与事务状态图 r4（统一owner生命周期，待审查）

## 所有权及状态

- Service是唯一事实与磁盘确认方；writer FIFO内AdvanceTo后调用。UI thread持有hub.sourceGuards[chain,trace]计数，每一个commandOnce实际accepted命令持一个lease。未准入／重复拒绝没有lease。
- presentation独立于lease：draft→confirmed-pending→closed。Cancel/Esc/new purpose/navigation只关旧展示，不撤销writer，不释放lease。pending目标冻结。
- 每个composer保留原window、surface、chain、trace、definition、native Handle及稳定host/key。普通输入状态与Add pending、ack尚未清空、真实Add failure正交。
- 独立recovery reason为none、saved-source、outcome-unconfirmed。它不是Add failure，不能伪造错误。reason按原source绑定，其他chain成功不得覆盖；同source已知deferred事实优先于outcome-unconfirmed。
- finalized evidence仅来自真正Finish，成功且非composing才有效；普通Snapshot证明当前内容，不证明旧native lease已撤销。重新mount/允许新编辑即废弃离屏旧Finish证据。

## 准入前及accepted

打开与确认走发起窗口titleTransition：注册title/subtask editor，Finalize composer native输入，不自动新增未提交文字。相关所有窗口同source readiness非空／pending／ack未清／failure／未知安全证明拒绝；其他chain不影响。只有commandOnce返回accepted才sourceGuard+1。之后可在其他窗口继续编辑；安全不依赖禁用重绘时机。

## Writer与publication

writer只访问Service，不读native Handle。成功publish confirmed图后，UI使用与真正composer渲染相同的actualSubtaskTargetEditable谓词，比较(previous graph, previous today)与(new graph, new today)。对本轮lease或已有恢复保护的source，旧可编辑→新不可编辑就是owner退场边界，不列举status；包括Complete、Return、AdvanceTo日期/锁日变化和lookup失败。旧node仍挂载、下一帧拆除之前，真正Finish受影响source composer：drain已交付字符、commit marked text到同binding、撤销旧native-client lease。未知／非空／Add pending／ack未清／真实failure保留原node为恢复；新confirmed source明确deferred才为saved-source，其他已确认状态改变只标outcome-unconfirmed（本次carryover未获成功，不能报延期已保存）；成功空且无其他阻塞可退场。后续无关publication不再Finish该恢复字段，不撤销它新的编辑lease。已有稳定恢复owner也不因第二个同源completion再次Finish；reason可按同source confirmed deferred单调升级，别source结果不串用。

## 每个已准入命令的UI completion

按以下顺序执行，不允许presentation失效导致早退漏处理：

1. 识别自己拥有的source lease和结果；Service返回空ID/error永不成功导航。
2. 若confirmed图已知同source deferred，则成功publication已保护原生生命周期；不重复Finish、不降级saved-source。
3. 若没有已确认deferred事实且command失败（包括AdvanceTo失败、before-reserve、after-reserve、after-persist未知、session stop），若confirmed source没有变化，不存在拆除owner边界，不做无必要Finish。对仍mounted的同source composer读取fresh Snapshot，只用于决定保留未知/非空/pending/ack/failure输入为outcome-unconfirmed，不宣称native lease已撤销；同一原node继续挂载，Close/导航以后仍须真正Finish。离屏composer仅用前次真正Finish证据。已有稳定恢复owner（saved或unconfirmed）不重复finalize；保持它新的焦点/marked lease。普通可见source仍是同一个原node，仅恢复说明替代普通提示；不渲染第二个同identity字段，不触碰别chain/native focus。
4. 此lease的原生保全完成（失败时保持原node和未知reason也算保全，不宣称输入已最终化）后才guard-1；剩余其他accepted命令仍持自己的计数。不能Cancel提前释放，不能错用全局bool。窗口可已detach，但detach仅允许此前真实Finish证明空或用户明确处理完输入；不再重建已关窗口。
5. 处理本presentation：只有window/hub/navigation/page/date/selection/purpose仍同一份时关闭原sheet；只有err=nil且非空ack ID才按原页面导航。已关闭／换目的的completion不能抢新页面或关闭新sheet。

## 用户导航造成原owner离屏

受guard保护的窗口从source切到其他chain/page时，既有titleTransition必须先真实Finish。当证明非空/pending/ack/failure时，离屏之前即记录outcome-unconfirmed独立恢复状态，不等publication。Finish失败直接拒绝导航，原node继续挂载。导航成功后原文本仍属同source，最小恢复入口即刻可见；不自动跳回、不拷贝进新source。空成功Finish证据可正常离开。没有guard且无恢复的普通transient行为不在本批扩张。

## Close/Quit在writer之前和之后

Close先执行原有native Finish／title save边界，再检查同source in-flight lease。相关composer非空／未知／pending／ack/failure拒绝detach，不设置自动closing；拒绝Quit不留下自动quitting请求。明确原生空白且无其他阻塞才可关闭。这样writer期间已交付但尚未binding的输入不会在publication到来前消失。

completion已发生后由recovery reason继续保护：未知仍可复制／明确清空；saved-source历史草稿同样可复制／明确清空。Finish失败保留native node，不能为成功导航而卸载。Close/Quit不等于静默丢弃本次已保护的输入。

## 恢复、明确清空与下一次合法操作

- 原生明确空白且可靠Snapshot/Finish证据可解除reason/真实failure，但不能清Add pending或未确认ack。
- before-reserve等失败后source仍editable：用户明确Return可提交原对象的Add；真实成功ack且native清空得到确认后解除reason，无需额外导航。carryover命令本身绝不自动重放。
- 已停止Service不因恢复入口而重试命令；只有用户明确Return才调用现有Add接口，Service拒绝仍保留原对象。复制/明确清空可用；不能据此宣称未知写入已回滚。
- 跨页面只显示最小原任务草稿入口，点击重查confirmed source identity并经titleTransition返回；正文不进sidebar/诊断。多source恢复彼此独立，顺序可达。新错误/notice不被恢复状态抢占。
- 合法新操作可重新准入；未解决同source输入依然阻断，明确清空/真实Add ack已解决则放行。旧命令completion只释放其自己的lease。

## 新证据要求

原五项独立原红保持原断言。补充所有四磁盘阶段writer期间B直接marked→completion→Close：未知不得称saved，zero automatic Add/replay，明确清空解除；before-reserve显式Return→真实Add ack清空解除；same source两不同target accepted计数2，第一completion后仍1，Cancel不减，重复拒绝不增；AdvanceTo失败、stopped/已detach空窗口completion不泄漏；成功source A不得把source B未确认输入标saved。

双命令独立门禁：第一条成功publication→第三窗恢复field重新focus/marked→第二条因源deferred失败→同native client仍能继续marked。失败completion必须跳过已建立稳定恢复owner，不能重演重复Finish抢焦点。

## 实际范围与缺失source

本轮普通taskService仅Add/Schedule/Complete/Return/Defer/Continue及既有子任务修改；没有parent trace/chain删除入口，unsupported delete菜单仍隐藏。新图lookup失败在谓词上仍是不可编辑，必须保留现有原node，不能标空；但缺失parent的跨页面恢复路由需要独立ghost-parent UI，不属于本次可达Service命令范围。若测试出现合法已准入命令可删除parent，不能以此范围说明跳过，须停止并扩充owner恢复合同；不声称当前支持任意外部图替换后恢复。

## 当前红证据

reviewer真实菜单先Complete/Return的两项原红，在第一publication后、Defer失败前已丢native owner；本地同样覆盖midnight和publication/ completion之间插入Frame。四fault晚输入completion→Close原红保持。作者新增guarded导航在writer完成前没有恢复入口原红。固定方案以共享渲染谓词+guarded用户导航Finish+稳定恢复owner三种路径覆盖，不在error回调猜测重建已销毁输入。
