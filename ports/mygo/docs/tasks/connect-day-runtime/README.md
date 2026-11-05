# 接通原版 Day Todo 结构与未来快速新增

task-id: connect-day-runtime。级别 P（新增写命令，沿用现有聚合事务）。2026-11-05。

## 需求与现状

用户要求原生Go+MyGo完整复刻，UI尽量与原版一致。现普通任务与月历真实接通，但Day仍通用65高列表、没有14日条且默认展示全会话统计。已读双方AGENTS、上游DayTodoPage/TaskCollectionPresentation/ReviewRail/自然日协调、原版day-todo.png与现有服务。原版默认flat/time/ascending，不将截图grouped状态误作默认，更不注入样例分类或统计。

## 实施前设计

- Day专用视图及纯taskview投影。14日期条以today−6为锚，负数floor14分页，非法边界保留nil槽不可点；严格验证图，真实pending点。默认time升序/pending优先、pin独立队列；历史标题只读引用定义。
- Header日期20semibold、星期12、状态胶囊；宽度不足采用上下两排；独立默认折叠280右栏。选中任务自动展开详情，未选中显示所选日真实只读统计/近7日趋势；未来不虚构复盘。未接通AI、复盘输入和视图配置不绘成可用控件。
- 条固定在滚动区外：10号星期、24圆、12号日、3号pending点，今日非选中独立描边；键盘focus可见且箭头不窃取输入。暂不宣称Swift弹簧动画/触控板手势/完整AX桥对等。
- 普通无分类行采用radius7、28完成控件、12间距、上下8，标题x52、13 medium/完成regular，按真实字体测量变高；不加原版没有的每行日期/状态副标题，不虚构标签/进度/子任务。
- Service.AddOnDate单次事务创建并排今天/未来；日期校验、today读取都在锁内。过去/锁定/失败不留下半创建或提前发布。UI经runCommand的自然日保护后使用当前选中日期，不误调用AddToday将未来任务写到今天。
- 目前仍保留明确“添加”按钮与Enter两种真实提交方式；原版从池选择器、日期弹窗和view菜单待完整接通，不能画成假按钮。短占位不承诺未支持的#/@/重复语法。历史/未来提示同样只描述真实能力。

## 风险、回滚与验证

不改v2数据格式，不开启其他生命周期/分类/同步。未来新增采用既有aggregate审计与append-only提交；写后不确定时不盲重试，重开核对事实。只回滚本次UI/服务入口，不删除或改写会话。仅新合成development资料，禁止production/iCloud/凭证/真实AI。

服务测试覆盖未来完整生命周期、一次revision、非法/过去/锁定、四故障阶段、并发和跨日锁。纯投影覆盖日期边界/负floor/跨世纪、priority与time排序分离、真实统计及nil趋势。UI验证未来新增→日期到达完成→跨日重开、草稿保护、自然日失败、只读导航图/磁盘SHA不变、长标题自适应/控件不误触、日期条固定/滚动、min size及rail开关。所有截图2×外置并查看，完整race/vet/gofmt+六目标编译+CLI；离屏证据不冒充三平台实机或Swift oracle。

## 实施与验证记录

- 已完成 AddOnDate 单事务入口、普通任务未来完整生命周期，以及日投影和原生 MyGo 专用页。Day 只读投影包含合法重复轨迹，Calendar 按上游排除；运行服务仍拒绝重复计划等未接通资料，不扩大接受范围。
- 所选日右栏遵照 ReviewRail：未来只有提示；其他日横排完成率、七日趋势、总任务标题下五状态行，零数值也保留。未接通 AI 和复盘编辑省略，不为凑截图填样例统计。
- 日期条内容57、pending槽9、底线与输入Y146通过源码/原版PNG核对；以主题匹配的真实字体测量标题和行高。任务标题x52、普通行44、圆角7、独立完成控件和hover均有交互/几何检查。
- 候选阶段复现并修复条件Header导致日期条失焦、长详情覆盖状态，见 GO-RUNTIME-005。长详情为独立滚动flow；完成点击不误选中，Tab箭头从真实焦点移动，导航和保存失败不丢草稿。
- 2026-11-05 聚合 scripts/check 通过：根包 race30.773s、taskapp5.273s、taskview1.040s，三领域模块及全部vet/gofmt通过。scripts/cross-build 六目标均编译，scripts/test-entrypoints 开发身份/create/reopen/reuse门禁通过。完整日志在外置构建根 evidence/day-{check,cross,entrypoints}-final.log。
- 另一审阅者独立只读检查未来服务/投影、最小960×720、日期与长详情滚动，通过后亲自查看2×复盘图。先前future统计疑点经上游noReview分支核对撤销，不添加原版没有的行为。
- 2×离屏图保存在外置 noonmark-day-build/evidence：strip、review、future、history、long-selected-min，均已实际查看；日期条/输入/右栏不重叠。截图是合成会话，不是正式Swift fixture。SF字体/图标、完整AX、动画/手势、主题/语言设置及实机尚未对等验证。
- 原版quick-add旁是任务池选择器，目前仍是明确可用的添加按钮。日期picker、视图配置、分类/标签/子任务编辑、复盘/AI与真实存储同步仍待接通。不得称本批等于完整复刻或发行版。
