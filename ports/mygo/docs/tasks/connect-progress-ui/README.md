# 接通真实加权进度

- task-id：connect-progress-ui
- 日期：2026-11-05（UTC）
- 级别：A，独立候选；正式整合由父任务负责。

## 需求与现状

在真实子任务 UI 上接通已独立审查的只读 ProgressProjection。严格保留 chain lineage 最大难度权重、历史 floor、completed parent manual 100%／1:1，不以当前已完成个数除总数替代，也不自动完成父任务。原版只读来源为 noonmark-reference@43e6eef；已读取 DayTodoPage.swift 的整个行布局及 showsProgress，TaskDetail.swift 的 DetailProgressSection、DetailProgressControl、ManualProgressSlider、StaticProgressBar，及 AppCopy+TaskDetail 的无障碍文案，并查看官方 day-todo.png。

## 像素与 API 方案（实施前）

- Day 行内：仅 pending／deferred／unfinished 且 0 < percent < 100。标题下方5pt；条150×4pt、Capsule 半径2pt；条与数字6pt；数字10pt semibold。使用已存在 sourceVisual.chip／accent，不加卡片、标题或装饰性边框。披露胶囊继续跟随标题 intrinsic 宽度，不因进度条变成固定靠右；行高给进度占位，展开 child 与下一行不重叠。
- Day 详情：13pt bold、等宽数字、右对齐；数字至条8pt；全内容宽6pt静态条、Capsule 半径3pt；100%用 sourceVisual.success，其余 accent。无障碍 RoleProgress，label 与原版一致并提供百分比 value／range。没有任何拖拽、键盘调整或 slider。
- Pool 不新增进度。manual mode 的持久数值照实只读显示；明确“手动调整进度尚未接通”。没有 setter 不创建假可写控件。源详情本地 subtasks.empty 与 engine 全链 guard 不同，本批不使用前者放行写入。
- titleHub 持有共享不可变进度缓存。创建会话读取确认快照一次；confirmed graph publish 内容变化时验证并重建一次，graph 与 projection 同帧发布。多窗口共享同一实例，所有 row／frame 只做 O(1) 查询；重复／失败发布不重建。无效图 fail-closed，不把错误当0%。同步 headless入口在创建及成功 command boundary 更新独立缓存，不在帧内计算。
- 只改 UI／publish 接线。不改 core、Service、validator、持久化、依赖或原 MyGo。暂不改变现有完成 command 的二次权限验证。纯投影约75ms／70MB的一年大 fixture 构造成本只允许在确认图边界承担，不能乘以窗／行／帧。

## 验证计划与边界

真实 Service 合成 fixture：难度改变、完成／撤销完成、删除、回池／重排期、重开、两个窗口、四阶段 durable failure、不提前发布。另以明确标注的有效合成历史图覆盖 lineage 和 floor 的 UI 直通，不声称这些未接通命令已实现。原生 MyGo Tester 截图、几何、颜色／圆角和无障碍检查；保留原子任务 IME／草稿／多行／短标题胶囊以及既有五条无障碍测试。最后运行 scripts/check、六目标 compile、Linux CLI。

截图只证明真实 MyGo headless renderer，不是 OS 实机／系统IME／Swift像素oracle。所有证据外置于 NOONMARK_MYGO_BUILD_ROOT；不运行 production，不读 iCloud／Keychain，不调用 AI，不 push。

## 最终候选

15项进度专项＋registry的race通过，完整scripts/check、六目标compile和Linux CLI均通过。真实截图与红绿故障证据见VERIFY.md。6个Go文件已冻结；手动调整保持未接通。只可按owned manifest逐文件整合，registry只追加本目录gates-to-append.tsv，不覆盖其他并行批次。
