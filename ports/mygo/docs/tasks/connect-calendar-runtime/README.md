# 接通普通任务只读月历

task-id: connect-calendar-runtime。级别 A。2026-11-05。

## 需求、现状与证据

用户要求完整原生跨平台复刻，尤其原版圆角、字体、颜色、间距和交互态。普通任务及安全 development 存储已经接通，但 Calendar 目前使用通用全历史列表，不是月历。已阅读上游 AGENTS、CalendarPage.swift、WorkspaceMonthGrid、GregorianCivilCalendar、自然日协调和真实 calendar.png，以及本端 AGENTS、DESIGN、当前服务/UI。源码基线43e6eef；这里只使用新合成 development 资料。

## 实施前设计

- 独立纯 Go taskview 投影先验证图，再依据 DayTodo 可见性/排序并排除重复链，提供周一起始、至少五行/必要六行的月格。保留空格边线；任务来自 trace 对应定义，热度是 min(completed,4)，不是完成率。投影失败显式报错，不伪装空数据。
- 运行 app 保存独立 selectedCalendarDate、折叠默认的248 DIP右栏及日期键盘焦点。月导航选目标月1号；今天回服务提供的 today；空日期和未来合法日期可选。选择不写磁盘，不改任务状态；自然日仍走已有事务，跟随今天的日历随其前进，手选历史不变。
- 新 runtime_calendar.go 负责绘制，不把原型任务注入 runtime。源码几何：左18、顶16、标题20/600、weekday11/600、七列无间隙、格高max(88,available/rows)、方格无圆角、选中左2 DIP蓝线、today独立4 DIP点；每格最多6条9.5字号、12高/2间距及真实+N，热度10×10/圆角2.5。
- 首批提供普通任务日期详情、真实计数、状态及“在 Day Todo 打开这一天”；完整续期/变更/风险洞察明确未接通。详情可展开/折叠，截图恰折叠不代表原功能没有右栏。长文本按原版截断/详情最多两行，格内绘制裁剪防止跨格污染。
- 箭头键只在日历日期具有焦点时移动±1/±7日，不能吞其他页输入；Tab/Enter/Space沿MyGo原生按钮机制。水平触控板手势、系统无障碍完整契约另待实机验证。

## 风险、回滚与验证

这是只读视图，不新增数据格式或命令，不扩大 Service 可接受状态。错误保留原始数据；撤回新view/projection调用即可回退，不回滚/删除会话。禁止production/iCloud/凭证/真实AI。

检查5/6行、全空月份、闰年和year1..9999、前后月/今天/日期键盘、today与selection独立、真实完成热度、6条及+N、历史title、长标题、折叠/展开、min size。运行真实UI新增/完成/跨日/重开→月格→打开Day Todo；反复只读导航前后目录与图精确不变。错误图不得展示0任务。新截图2×外置，仅作离屏证据；全模块race/vet/gofmt、六目标编译及CLI回归。无DISPLAY、Swift oracle或三平台实机，不宣称原生/像素全等。


## 开发复现与必要适配

独立检查在候选中复现GO-RUNTIME-004的三项问题：自然日失败消息遗漏、六行小窗口预览与计数裁切、Tab焦点不可见。已经登记症状门禁。密集小高窗现在根据实际格高显示最多6条并预留可见+N，其余均可在详情查看；这是明确防裁切适配，不声称逐字复制Swift始终prefix(6)。源码预览目标为12高加2间距，但MyGo/Linux实际Text行盒可达15/14高；仅设FixedLineHeight不能缩小字体包围盒。现在用匹配字号的透明零尺寸Themed测量域取得真实占用，不改根主题；节距取max(14,真实高度)，计数预留真实高度。默认主题直接MeasureText的base strut会高估，测量与实际Text高度相等已设门禁。键盘使用MyGo原生可见focus，与原版专用焦点桥有平台差异。

Pane toggle采用独立radius6/透明idle，HeaderButton保留radius7/panel2；Open Day Todo为原版11.5medium蓝色text action，详情status为独立18高胶囊，不套通用导航配方。完整洞察仍明确未接通，不伪造百分比或风险。


## 验证结果

2026-11-05：完整 scripts/check 通过，根 UI race 18.934s，taskapp/taskview 及三个领域模块 race、vet、gofmt 均通过；六目标完整 App 重新编译通过，实际 CLI 创建/重开/身份保护通过。独立补测的root race亦通过，原3个症状转绿；没有弱化可见性断言。只读导航/选日/月切换前后任务图及逐文件SHA256一致，重开后的completed/unfinished与空分类audit保持一致。

最终离屏测试及6张2×图在外置 /tmp/noonmark-calendar-build/evidence：五行、六行、最小窗口、展开详情、密集六行、边界失败警告。实际查看渲染图，密集末条与计数无重叠，source五行仍6+2，六行小窗4+4。完整 root/calendar 症状日志及独立验修日志均外置，不上传运行资料。

仍未验收：真实Linux桌面、macOS/Windows/arm64实机、Swift oracle、原版触控板横向手势及完整无障碍桥、字体/SF Symbols像素等同、完整续期/变更/风险洞察。只交付普通任务只读月历，不把其他生命周期或分类能力默认接通。
