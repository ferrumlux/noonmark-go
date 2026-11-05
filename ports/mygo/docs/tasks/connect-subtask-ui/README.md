# 接通普通子任务原生 UI

任务：connect-subtask-ui。2026-11-05，独立候选；不改原版、正式树或 core/Service 候选。

## 需求与现状

用户要求 Go + MyGo 原生 UI 尽量与原版一致，包括圆角。来源为 Swift 43e6eef 的 DayTodoPage.swift:690–850、1034–1238，TaskDetail.swift:83–108、PoolDetail.swift:77–177，SubtaskDifficultyDotMenu.swift:4–83，MarkdownEditing.swift。实施前，既有专用 Service 已含 planned/actual 子任务事务，真实 UI 尚未使用。本候选接通这些原生交互；最终验证状态见 VERIFY.md。复用已校验的 title session、原生 finalization 和共享串行 writer，不扩大空 catalog、undo 或导入范围。

## 像素与实施方案

- Day checkbox 15×15，圆角5、描边1.5，完整命中28×28；完成勾9pt。Pool 同图形，不创建完成按钮。
- 难度点10×10、描边0.75、line2 opacity0.65，命中12×28，无箭头原生菜单；简单/中等/困难三项与当前勾选。颜色为text3×0.58、accent×0.72、warn×0.92。
- HStack spacing4。只读标题12pt；实际 native editor 遵循 MarkdownEditing.swift:24–26/46–62/86–91 的12.5pt medium、垂直inset8.5、最小32/最大120。Day 删除trash10pt、命中28×28，Pool删除xmark；行距6。composer 背景为源 Theme.panel2，无额外描边，圆角7。
- Day list/detail 分别使用 day-list/day accessibility namespace，owner=`subtask:<完整标题surface identifier>`；对象路由包含 chain、trace 或 expected definition、child，不根据当前选择重新定位。
- list 仅展开/收起现有子任务。计数胶囊使用22pt高度、横向7pt留白、9pt chevron、4pt间距和11pt semibold计数，宽度真实测量。胶囊贴随内容 intrinsic 宽度并加12pt accessory spacing，短标题不靠右；长标题受可用宽度约束。chevron由9pt原生SVG绘制，不用字体字符近似。详情新增 composer 圆角7，Return 提交，成功精确确认才清空，失败保留。现有标题700ms、失焦/Return、导航/关闭最终化复用 coordinator。
- UI 只读共享 writer FIFO 发布的快照；不在 UI 上等待磁盘。删除、父完成、回池、排期通过生命周期 flush，旧回调不指向新选择，重复命令跨窗口合并。
- 不接子任务备注、分类、abandon、重排或父自动完成。完整链 lineage 的加权进度尚未移植，不以简单个数或伪造百分比代替。

## 风险与验证

多表面同 child 的两个 native binding 必须独立，持久事实同步不能覆盖任一 dirty 草稿。动作可能使另一窗口编辑变只读，应保留失败草稿，禁止悄悄销毁。针对真实 MyGo Tester + Service 验证700ms、IME、Return、旧回调、导航/关闭、故障、双面/双窗、重开及 future/history。截图来自真实 MyGo headless renderer，不是 CSS mockup，也不是 macOS/Windows 实机或 Swift oracle。

仅 development/e2e 合成资料；无生产/iCloud/Keychain/真实 AI 网络调用。外置构建根 /workspace/shared/noonmark-subtask-ui-build-20261105；通过 scripts/environment 加载冻结依赖。正式整合由父任务负责。

## 已阅读的布局验收源码

SubtaskLayoutE2EAutomation.swift:970–1085 对 Pool 和 Day 均要求真实文本形成多行；难度控件宽度≤14.5、标题占行宽至少63%、标题至删除间距3.5–4.5、横向text inset/line padding≤0.5、行高容纳editor、行和composer不相交。本候选据此保留 Pool 原生多行，不误把 SwiftUI `.lineLimit(1)` 当成单行领域限制。这里是静态源码对照，未运行该 Swift E2E。

## 文件与整合

Go owned files：main.go、runtime_title.go、runtime_day.go、runtime_ui.go、runtime_subtask.go、runtime_subtask_test.go、runtime_subtask_independent_review_test.go、runtime_subtask_recovery_review_test.go、runtime_subtask_r3_review_test.go。没有修改 Service、core、titlecoordinator、titlesession、依赖patch或三个冻结领域模块的go.mod。

新增本目录文档，以及 GO-UI-SUBTASK-001 至009故障案例和 gates.tsv 九行。父任务另有 core/Service 的新增审查文档与门禁；整合须追加本批九行，不用整个 gates.tsv 覆盖父任务。Go 冻结清单在外置 evidence/frozen-go-manifest-r5.json。
