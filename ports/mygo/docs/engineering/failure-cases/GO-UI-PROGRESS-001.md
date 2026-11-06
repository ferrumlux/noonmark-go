# GO-UI-PROGRESS-001：行内进度的无障碍与点击边界

状态：已修复（源码与本地门禁），修复提交 f4fbd80fbd9991688d7c0b7c80c4f1d46d071625。Git author／committer 为 Codex <codex@openai.com>；远端写入与该提交的GitHub CI仍待确认。首次发现：2026-11-05 21:52 UTC。

- 症状：首轮进度条可见、详情无障碍正常，但收起详情栏后，行内 RoleProgress 无法被读屏树找到。移为平级元素后读屏可达，却截获了原本用于选中父任务的点击。
- 引入：connect-progress-ui 未提交独立候选，没有引入 commit；Git author／committer 不适用。候选由Codex开发，真实自然人操作者未知。正式源码未接入有问题的候选版本。
- 取证：progress-second-focused.log 中 TestProgressUIListAccessibilityAndSelection 明确找不到 RoleProgress。MyGo access.go 的 leafRole 将 RoleButton 视为无障碍叶子。progress-third-focused.log 记录同一症状门禁的进度区域点击未选中父项；runtime.go 的 hit-test 及 IgnoreInput 继承规则确认普通只读 Box 仍可遮住底层点击。
- 根因修正：真实进度为父选择按钮的平级原生 RoleProgress，不嵌在叶子按钮内部；整个进度 subtree IgnoreInput(true)，保留读屏标签、value／range，鼠标与键盘穿透到原行。位置补偿现有选中描边 inset，不改父标题文本的 Label，不加假按钮或复制原生动作。
- fast gate：TestProgressUIVisibilityMatrixAndSourceTokens、TestProgressUIConstructorOnlyAtPublishBoundary。
- symptom gate：TestProgressUIListAccessibilityAndSelection、TestProgressUISourceGeometryAndHeadlessCapture。完整 scripts/check 强制运行。
- 保留范围：子任务 checkbox、difficulty、composer、草稿、父完成与选择仍走原命令路径。测试没有修改已有五个标题/无障碍门禁。
- 回滚：只回滚本 UI owned 文件，纯 core／Service／资料格式均未修改。没有发行物；真实 OS、Swift 像素及系统读屏尚未验收。
- 教训：截图可见与读屏可达是两条不同契约；被动展示元素必须同时验证命中穿透。正式 commit、最终全部门禁及独立审核完成后才关闭案例。

2026-11-05 22:00 UTC：原症状门禁与全部15项进度专项race通过23.720s；相同冻结Go源码完整scripts/check exit0通过（main race112.591s），六目标compile及Linux CLI也通过。候选没有已知阻断；仍待独立审核／正式整合commit，因此状态保留处理中。

2026-11-05 22:12 UTC：独立审核完整门禁和5个新增永久测试通过；分别用外置overlay恢复按钮叶子隐藏与点击遮挡，原症状真实判红，修复代码为绿。正式整合完整scripts/check退出0（main race129.591s），六目标编译和Linux隔离CLI通过，修复commit已固定，故关闭源码故障。未因此宣称OS读屏实机、Swift像素对等或新远端CI已通过。
