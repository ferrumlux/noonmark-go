# GO-UI-SUBTASK-001：子任务计数测量重用同一 native key

状态：处理中，候选修正待正式整合 commit。首次发现：2026-11-05 19:59 UTC。

- 症状：隔离候选将展开胶囊改为源 intrinsic 宽度后，首次新增子项即由 MyGo duplicate-key 断言中止渲染。未发布、未触碰生产。
- 引入位置：本任务未提交的 runtime_subtask.go/subtaskDisclosureWidth 与 runtime_day.go 调用。不存在可引用的引入 commit；Git author/committer 不适用，实际执行修改为本轮 UI 工作线程。
- 错误机制：尺寸测量也创建 MyGo Themed 元素。同一个 row scope 两次用固定 key 测量、以及多个父行在 list scope 用固定 key，都重复绑定同一 native 状态。不是存储、子任务 Service、字体或 MyGo 依赖失败。
- 红证据：外置 evidence/subtask-disclosure-duplicate-key-red.log，TestSubtaskUIDayCommandsAndReopen，明确 duplicate key=subtask-disclosure-width。无任务正文或用户资料记录。
- 根因修正：每个父项测量 key 加 trace identity；同一 row 中重用已得 intrinsic 尺寸，不重复创建测量元素。
- fast gate：TestSubtaskUIDisclosureUsesObjectScopedKeys，三个父项具有相同子项数量；真实路径 symptom gate：TestSubtaskUIDayCommandsAndReopen，Service 新增后 native 重绘及后续编辑。
- 聚合入口：scripts/check；gates.tsv 登记。仅本地 headless MyGo 渲染验证，不是 Swift/OS oracle。
- 发行与回滚：父任务整合前必须重跑完整 gates，确定修正 commit 后更新状态。回滚仅撤回本批 UI 文件，不覆盖任何用户 snapshot。
- 教训：原生测量控件同样需要对象和调用表面隔离；不能把只读 measure 当无状态函数，尺寸应单次测量后传递。
