# GO-UI-SUBTASK-006：子项计数胶囊错误固定到行右边

状态：处理中，候选修正待正式整合commit。首次发现：2026-11-05 20:13 UTC。

- 症状：短父标题后出现过宽空白，count胶囊固定在所有任务行最右边，与官方day-todo.png和Swift的内容后Spacer不同。
- 引入：本轮未提交runtime_day.go；无引入commit，Git author/committer不适用，实际修改为UI工作线程。
- 来源：DayTodoPage.swift:690–823，HStack含completion、intrinsic内容VStack、count accessory、最后Spacer(minLength:0)，间距taskRowAccessorySpacing=12。不能只把胶囊本身宽度做intrinsic。
- 原红：TestSubtaskUIDisclosureFollowsIntrinsicContent，evidence/r3-recovery-layout-red.log，短标题capsule.X-row.X=580。
- 修正：测量父标题真实intrinsic宽度，按可用空间收缩；capsule跟随内容+12pt。选中行border对内部布局的1pt inset计入几何，不令长标题越右界。chevron9pt用原生SVG，不以›/⌄字形代替SF Symbol。未来分类/进度真实接入时必须将内容VStack最大宽度计入。
- fast与symptom gate：TestSubtaskUIDisclosureFollowsIntrinsicContent，覆盖短与长标题的实际native边界和12pt间距；TestSubtaskUISourceGeometryAndCapture。聚合scripts/check。
- 教训：从源码的布局顺序和真实渲染验证相对位置，不能只核对单控件的常量。MyGo向量和系统字体差异仍非Swift像素oracle。正式commit后再关闭案例。
