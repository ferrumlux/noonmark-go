# GO-UI-SUBTASK-009：几何验证标签覆盖父任务标题文本

状态：处理中，候选修正待正式整合commit。首次发现：2026-11-05 20:31 UTC。

- 症状：子任务专项与独立审查38项通过，但完整scripts/check中五个既有Day/通用运行UI测试报告任务消失、创建后找不到、日期恢复不符。领域持久事实没有丢失。
- 引入：r3的runtime_day.go给父标题ui.Text加了仅为新几何测试定位的Label，原native文本lookup/无障碍内容被自定义标签替换；无引入commit，Git author/committer不适用，实际修改为UI工作线程。
- 原红：evidence/final-check-r4.log的TestLiveDayFutureQuickAddAndRestart、TestLiveDayDynamicRowsAndIndependentCompletion、TestLiveDaySourceGeometryHoverAndFocus、TestLiveUICommandsPersistAcrossRestart、TestLiveUIDateAndUnsupportedPagesDoNotInventData。
- 修正：移除该非必要Label，父标题继续作为真实可访问文本；新几何门禁通过实际短/长标题寻找原生rect，不改既有5例断言，不引入测试专用UI。
- fast gate：TestLiveDaySourceGeometryHoverAndFocus。symptom gates：上述其他4个TestLive门禁及TestSubtaskUIDisclosureFollowsIntrinsicContent。聚合scripts/check。
- 教训：测试便利不能改变正常无障碍语义；专项通过不能代替完整回归。正式commit后再关闭此案例。
