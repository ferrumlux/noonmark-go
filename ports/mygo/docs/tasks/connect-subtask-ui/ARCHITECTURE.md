# 子任务 UI 边界与保留草稿

## 对象与表面

`runtimeSubtaskTarget` 捕获 chain、trace 或 expected definition、child。只把这组不可变身份编码为 coordinator 原有 opaque target，既不改专用 Service，也不重新根据当前 selection 查写入目的地。Day list 的 owner 是 `subtask:day-list.subtask.<ID>.title`；Day detail 是 `subtask:day.subtask.<ID>.title`；Pool 是 `subtask:pool.subtask.<ID>.title`。窗口仍由独立 window ID 隔离。同一 child 的两个 owner 有独立 native binding、registration、generation、draft、ack。

Pool 路由冻结 expected definition。合法父标题版本分叉使旧 editor 锁定，不把旧子项草稿沿 chain 静默转写新 definition。实际 child 绑定 trace 与 child ID，父标题确认不会确认或清空 child 草稿。

## 输入与生命周期

编辑器仍使用已冻结 MyGo Handle 的 TextInputSnapshot/FinishTextEditing，不能以应用 draft 替代真实 native proof。700ms timer、composition、空白恢复上次 acknowledged text、精确存储 ack、失败重试与旧 generation 拒绝都来自既有 coordinator/session。Return 通过 native OnTextKey 提交；Shift+Return 保留原生换行能力。普通 blur 与导航/关闭/退出的生命周期路径保持区分。

`titleHub.register` 注册父标题和全部 child surfaces。导航只在整个窗口的注册集合完成 native finalization、精确存储确认及 disposal 后切换。关闭和退出仍拦截实际 native 回调，等待所有 writer command 及 title session，不用超时绕过。自然日转换先读取已送达 native 输入，然后由 Service 日期守卫阻止回填过去日。

新增 composer 不自动变成新领域事实。只有明确 Return 才调用 Add，完整事务成功后才清空相同 native text。较新输入、marked text、失败和离屏期间的草稿均保留；离屏成功 ack 在再次挂载后取得 fresh native proof 才清空旧提交文本。尚未提交的 composer 按对象保存到窗口的草稿表；不承诺跨关闭/重启恢复未提交的新项。明确 Add 失败则是另一种状态：普通Close不视为丢弃许可，Close/Quit检查所有失败composer（含离屏），重试成功或fresh native证明用户明确清空才释放。这是端口失败防丢稿守卫，不冒称Swift transient composer原本具有完全相同的关闭语义。

## 共享事务与发布

所有子任务 Service 调用复用同一个 writer lane。UI 只读取 titleHub 已确认的 graph clone，不等待 Service 锁。存储事务完成后，graph/today 的发布与标题完成回调在同一 FIFO，不能以延迟 Snapshot 替代该事务 ack。

原生菜单host还绑定不可复用的 *titleWindow 实例，防止旧native callback跨Service Close/Open和同app重挂。显式命令 key 包含不可变目标、操作及所选目标值，抑制 pending 期间的重复激活，包括两窗口同时完成同一 child。选择难度与删除不是循环按钮；原生菜单选项为简单/中等/困难，并显示当前项。跨窗口重复新增的第二个未执行请求不自行声称成功或丢弃其 draft。

## 被另一窗口改变的对象

另一窗口删除或回池可能令旧对象不再可编辑。先把最新 native snapshot 读入 session，再锁定写入。未确认草稿继续挂在原 owner；若原子项已经不在 presentable graph，保留原有只读编辑表面和错误，不能把该文本丢掉或放到新的 child。Day list 的原父行只为这类保留草稿维持显示；无 dirty/flight/error 的 editor 则通过正常 final proof 清理。已完成但父项 pending 的子项仍可编辑，不额外锁定。

## 明确未对等的部分

- 完整 chain lineage 加权进度和历史 floor 尚未接入，UI 不显示以简单计数代替的进度。
- 分类的非空 catalog、独立子任务备注、撤销恢复、拖拽重排、低层 abandon 不在本批。
- 子项全完成只解锁现有父完成能力，仍须显式完成父项。
- 原生 MyGo 控件可运行测试不证明系统 IME、macOS/Windows 窗口或 Swift oracle 已验收。SVG chevron/trash/xmark/lock 是原生向量绘制，不是 Apple 系统字体像素等价声明。

## 失败新增的可达恢复

失败 composer 的原 surface、trace 或 expected definition 不可变。另一窗口把父项回池、完成、推进到历史或分叉definition后，同chain的当前Day/Pool详情仍显示原日期或原任务池版本的恢复输入区；原Day父行也保留，用户可回到原日期找到草稿。恢复区只为失败明确提交显示，普通成功界面仍按Swift源布局，不发明普通“恢复”按钮。当前正常新建composer暂不与失败输入并排，以免把旧输入误当新对象。

原对象若仍可写，Return重试原目标；若已失效，Service拒绝，用户仍可复制或原生明确清空文本，不会转写新trace/definition。渲染位置变化保留原native键；页面切换前完成已有native finalization。尚未提交的marked text通过真实TextInputSnapshot的Text/MarkedText核验，不能以应用draft证明安全。清空只有fresh native已不再composing且文本空白才释放关闭屏障。

错误文案使用“添加尚未确认”，不能声称“未添加”：after-persist故障可能已有持久事实但未获成功确认。清空仅丢弃用户明确清空的失败草稿，不删除已经持久化的子项，也不绕过blocked Service。

被删子项的保留草稿是恢复资料，不能算入持久可见子任务数量。披露胶囊计数取actualSubtaskRows已确认事实；仅剩ghost时显示“未保存草稿”，行内也标明“原对象的未保存草稿”。父行临时保留不修改日统计、领域事实或进度。
