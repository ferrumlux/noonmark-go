# 接通从任务池排期选择器

task-id: connect-pool-picker。级别 P（新增实际写入口，复用已验证单任务排期事务）。2026-11-05。

## 实施前设计

用户要求完整Go+MyGo复刻并尽量保持原版UI。已核对 WorkspaceSheets.swift:89–153、DayTodoPage.swift:330–348、NoonmarkStore+TaskMutations.swift:633–658 及 taskPool/isInTaskPool 源码。

- Day输入仍以Enter快速新增，右侧恢复原版“从任务池排期…”而不是额外添加按钮。使用源HeaderButton、字号12、圆角7与输入间距8，真实行为替换已记录的差异，不改任务池页面现有添加按钮。
- 从池sheet宽400、padding16、spacing12、标题14semibold、列表最多320高、列表项spacing6/padding10/radius7，左标题右排期。没有虚构搜索、多选或样例列表。实际Swift平台sheet阴影/背景与MyGo DialogBase不同，需明确非实机对等。
- 纯ProjectPool严格验证图，按当前definition.createdAt升序、active/非recurring/无pending且latest事实资格，返回真实任务。排序同刻仅加确定性ID顺序；上游Swift comparator未定义同刻tie，不声称此处oracle一致。
- MyGo DialogBase提供模态屏蔽、Escape/外侧取消及焦点圈；初始首项/空表取消焦点，长列表独立滚动；Day快捷键和提交在modal期间不活动。取消/导航只读且保留quick-add草稿。
- 单击排期复用runCommand自然日保护与Service.Schedule；成功选中新trace并打开详情，失败保留sheet与原任务、明确错误。比原Swift在调用返回后无条件关sheet更保守，避免错误被遮住；不盲重试写后不确定结果。
- 无新存储格式或批量命令，不读取真实账户资料。继续只用新合成development会话。

## 验证计划

纯投影表驱动完整资格、排序、历史标题与无alias；UI空表/首项focus/Escape/外侧/按钮取消/重新开、今日未来成功→重开、重复点击仅一次、后台自然日与历史拒绝、损坏失败原图/草稿不变、模态阻止背景日期/输入与滚动长列表、960×720中文长标题真实可见矩形。通过全race/vet/gofmt、六目标编译和CLI，并查看2×PNG再发布。真实Swift oracle和桌面窗口仍待具备环境后验证。

## 完成记录与准确边界

- 已实现严格纯ProjectPool（8组单测）、原版单击排期的模态列表、成功后选中新trace与展开详情、空表/初始focus/取消/历史保护。复用原Service.Schedule，不新增数据格式、命令或生产接受范围。
- 快速新增改为真实编辑器Enter路径，旧回归测试也改为先聚焦编辑器再Return；任务池页添加按钮有独立回归，没有隐藏替代动作或绕过界面的测试。
- 对话框开着时，背景指针/日期键/文字输入不可写；取消、Escape、外侧点击不排期且保留草稿。自然日更新后仍对当前可见所选日期执行既有runCommand保护，边界失败不排期。
- 写失败保留弹窗、原图及草稿；错误文本在sheet内有独立可访问标签和实际矩形断言，避免测试误匹配背景同文。损坏/冲突/写后不确定仍服从既有存储冻结，不额外重试或清理资料。
- 截图保存直接取完成交互的MyGo Tester状态并设2× device scale；不再用新ui.Render当作已滚动/已聚焦状态的证据。外置evidence三图覆盖第2项Tab焦点、960×720滚到底部17–21项、失败消息，均已查看。初始首项焦点另由真实tt.Focused断言验证。
- 上游公开截图没有此sheet的实机图，不能声称sheet外壳像素一致。源内容宽400/padding16/gap12、标题14、列表最高320/间6/行padding10/radius7已实现；MyGo自有DialogBase外壳radius12/遮罩/阴影、系统字体/Markdown内联与SF图标差异仍明确未对等。
- 原型20场景不变；生产/iCloud/AI均未调用。真实macOS/Windows/Linux窗口和Swift oracle仍需具备环境后验收。

最终验证：2026-11-05，scripts/check 根包race34.508s，所有taskapp/taskview及三领域模块race/vet/gofmt通过；scripts/cross-build全部六目标编译；scripts/test-entrypoints开发身份/create/reopen/reuse保护通过。外置构建日志为 evidence/picker-{check,cross,entrypoints}-final.log。上述编译不等于各平台实际启动。

独立审阅：额外4组overlay检查覆盖22行Tab与自动滚动、背景输入隔离、取消重开首项、1000字中文标题、故障前后图/磁盘签名、960×720至1600×900可达性，未发现阻断性缺陷。已确认P3视觉差异：当前标题仍直接显示Markdown原文，而Swift用MarkdownInlineText；应与其他标题面统一实现富文本，不将本批宣称为完全视觉对等。
