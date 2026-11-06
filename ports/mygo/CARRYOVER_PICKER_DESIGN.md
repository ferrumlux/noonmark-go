# 延期／延续日期组件（独立批次）

## 需求
复刻原版中文 ordinary-task 日期 sheet；只产出 draft/取消/确认 intent，不接菜单、Service、持久化或导航。调用方以后必须先经过现有 native editor 保存边界，再验证 intent 身份及最新任务资格。

## 现状与来源
正式基线 8c68c3b；Swift 43e6eef 的 DatePickerSheet.swift、NoonmarkStore.swift:555–653、AppCopy+Workspace.swift:152–201、MacUIDesignContract.swift:118–135、Theme.swift:70–96、WorkspaceUIPrimitives.swift HeaderButton 为直接源码依据。现有 runtime_calendar.go 与 runtime_pool_picker.go 使用 MyGo DialogBase、FocusBind、Shortcut 与 sourceVisual。未找到日期 sheet 专属真实 App 截图；不能以 headless 结果宣称 Swift/native 验收。

## 像素规格（先于实现）
白色内容宽340、padding18、内部304。竖排 gap14；标题16/600、hint11/400、间距4；calendar gap8；月份13/600，header28；weekday11/600、18高（tracking0.6，MyGo LetterSpacing(.6)）；7列×6行，每格40×30、列距4、行距2，邻月可见。selected circle24，today未选中描边 accent×0.78/1，hover圆角6；文字11.5，selected600其余400，禁用line2×0.72。footer分隔线与内容距12、按钮gap8、高28；summary11.5/500、错误warn色。sourceVisual 复用 coolGray 色值。系统 sheet chrome/按钮/font shaping 不能冒充原版：暂用项目共享外观的 Overlay+Modal shell radius12、shadow、系统外框近似，标记为待实机验收。没有 X、额外 toolbar 或确认窗。

## 设计与边界
窗口/session/来源 chain+trace/来源上下文/打开次数 token 必须完整相等；取消与确认只能消费当前打开实例一次。选日、切月仅修改本地值；确认时用调用者最新 today 重验跨午夜。Defer初值明天且>today；Continue初值today且>=today。today仅允许时显示；左右±1、上下±7；不能去最小月份之前。无业务日期上限；Go既有 LocalDate 的1–9999表示范围与Swift差异必须明示，不修改核心来掩盖。边缘网格不可表示槽保留空白并禁用，保持42槽。

组件不持有 app 或 Service；每次呈现接收当前 token 与 today，返回带token的单次intent。调用方负责再验证 native focus/IME、navigation/source、跨窗串行化、保存后readback及错误；本批未接通任何普通任务操作。MyGo焦点、可访问标签及键盘仅通过headless验证；系统读屏、IME、缩放/高对比待native。选中进度使用公开AnimateTo(120ms, EaseOut)，驱动圆形fill/today描边/文字颜色；该API会尊重ReduceMotion并从首个target初始化，避免首次透明动画。实测clock 0/60/120ms像素与ReduceMotion立即终态；Swift曲线及OS仍待验收。

## 验证
实际 MyGo node/hit area、42格、默认、闰年跨月、最小月、±键盘、Cancel/Esc/Return、重复确认、旧token、替换purpose、跨午夜；外置2×PNG肉眼检视。完整root Go测试回归，不调用live服务。

## 实施校正
直接核对 MyGo ui/base.go:666 后，没有采用 DialogBase 的背景点击关闭行为；使用公开 Overlay + Modal 原语，背景点击不关闭。Enter 用 panel.Shortcut 注册元素级默认动作，日历 focus在选日与切月后恢复；实测日期、月份箭头、取消、确认焦点均只发一次确认，外层快捷键不消费。weekday tracking使用已核实的LetterSpacing(.6)，日期格式据AppDateTimeFormatterTests使用“周四”等，不使用“星期四”。summary使用SingleLine的省略号。

初轮候选测试真实发现 Enter 被日期 Button 的默认 click吞掉，两个断言为红；这是本批未集成候选的问题，非正式8c68c3b缺陷。相同断言修复后转绿，并新增各焦点与外层快捷键回归。没有修改MyGo框架或全局键规则。

箭头仅在calendar焦点拥有者响应；Tab移到footer/header不移动日期（依据DateNavigationKeyboardFocusView.keyDown只由first responder收键）。today描边仅在未选中时出现；selected-today对照像素测试与截图覆盖。MyGo Selected立即设置默认背景，故调用Selected后显式Background(ui.Transparent)，保留MyGo Checked状态且没有整格蓝色；真实Access树为RoleButton、Checked=true、Selected=false，不能声称Swift isSelected已对等。中文Value仅补充，原生Selected红仍待独立依赖适配；Value中文为补充。标题RoleHeading、星期装饰AccessHidden均按Swift对应语义处理。Shift方向键与无modifier相同，Cmd/Ctrl/Alt不处理。

## 未通过的明确门禁
独立审查`TestReviewCarryoverNativeAccessibilitySemantics`的native Selected断言仍为红（MyGo Button只导出Checked）。不改角色伪装ListItem，不改原框架；这批候选不能因此宣称整体原版可访问验收完成。自身KnownAccessibilityMapping测试只记录真实框架状态，不是把Checked当作Selected通过。


## 正式整合更正（2026-11-06）

上述是原独立组件的历史冻结记录，保留原红与限制。本批独立修复的固定 MyGo patch 0005/0006 已让同字节原 Selected 症状断言转绿，并保留 RoleButton、Checked 独立状态、像素与键盘行为。正式整合仅将旧 KnownAccessibilityMapping 缺陷诊断明确改名为 ShiftAndSelectedAccessibility，要求 Selected=true、Checked=false；没有删除独立原红门禁。具体正式验证结果见 docs/tasks/integrate-carryover-picker/VERIFY.md。纯组件仍不调用 Service，业务菜单尚未接线，真实系统读屏和 Swift 像素验收仍未完成。
