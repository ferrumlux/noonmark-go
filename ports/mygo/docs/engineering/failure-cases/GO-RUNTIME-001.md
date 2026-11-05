# GO-RUNTIME-001：初次持久化接线的重开一致性与速记目标

状态：处理中，候选修复测试已转绿，修复 commit 待填写。

- 首次发现：2026-11-05，本轮新代码的集成测试；尚未发布该运行路径。
- 引入 commit：尚未提交的本地候选，不属于 8b273d93 首批备份。实际改动者为本轮开发执行记录；Git identity 不能独立证明操作者。
- 症状一：内存 nil slices 经 wire 编解码变成空 slices，重开后 reflect.DeepEqual 不相等；Instant 与任务事实未丢失。
- 根因修复：首次创建和每次提交前统一按候选 codec 规范化，持久化成功后才发布相同规范化快照。
- 症状二：快速记录按钮与静态文案使用完全相同的可访问名称，MyGo Tester 选中非交互文字，提交操作无法定位。
- 根因修复：可视短文案为「保存」，可访问名称为「保存到任务池」，保持真实控件唯一语义目标。
- 回归：TestCreateScheduleCompleteUndoRestart、TestNativePayloadIsNotSwiftImport、TestLiveUICommandsPersistAcrossRestart、TestLiveUICaptureAndSharedSession 由红转绿；均由 scripts/check 强制执行。
- 风险与回滚：仅新增 development 路径，未触碰生产；首批静态原型入口保留。真实桌面窗口/系统输入法未运行，不冒称完成发行门禁。
- 教训：比较整图重启一致性应在 UI 发布之前归一化；所有可操作目标须使用稳定 UUID 与不歧义可访问名称。
