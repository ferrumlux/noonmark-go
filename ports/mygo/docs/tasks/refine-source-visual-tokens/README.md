# 收敛原版视觉常量

- task-id: refine-source-visual-tokens
- 级别：A；日期：2026-11-05。
- 需求：在原生 Go + MyGo 视图中尽量贴近原版，特别是圆角、字级、色彩及侧栏；不改变真实任务事实或交易边界。

## 现状与证据

已阅读 AGENTS.md、DESIGN.md、外置 visual-parity-audit 的 README 与 source_token_inventory.json；已实际查看 Swift Day Todo、旧原型 Day Todo、真实 runtime Day 三张 PNG。原型与 Swift 为 2400×1536（2×），旧 runtime 为 1200×768（1×），不得据此声称像素等同。DESIGN.md 的旧色板是近似值，本任务以当前 Swift 源码表达式为权威。

## 实施前精确设计

1. 新建 visual_tokens.go，保留 Theme.swift:71–94、184–209 的 coolGray 正常对比度 RGB 小数，统一舍入到 MyGo 8-bit sRGB。主文字 (.11,.11,.12)、次文字 (.42,.42,.46)、强调色 (.16,.38,.78)、选中色 (.93,.955,1)、侧栏 (.982,.981,.978)。白 panel 和近白 background 分别命名，绝不混用；不冒充 warmPaper / increasedContrast 已实现。
2. 侧栏依 WorkspaceShellViews.swift:145–186：行高34、水平边距9（compact7）、圆角8、图标14置于19宽框、图标框与文字间距10、leading14，标签14.5，未选中 medium500/text2，选中 semibold600/text1；count11/text3。选中淡蓝乘默认 layeredSurfaceOpacity .78（AccessibilityPresentationPolicy.swift:35–37）；减少透明度尚未实现，不假称适配；hover 为 panel alpha.72 和 line alpha.45，不再通用蓝色边框。导航语义色采用 Theme.swift:198–209 的各页面独立色值，不混同一般 accent。
3. PageHeader.swift 标题20/600，DayTodoPage.swift 标题13/500、完成13/400；完成文字用 Contract MacUITerminalTaskTextColorMetrics.coolGray (.627451,.627451,.658824)。只调现有 Day/侧栏/任务外观，不加入不存在的分组、标签、子任务。
4. 保持各组件独立半径：导航8、分类徽章3、任务完成圆18直径/9半径；不将日期圆、胶囊或编辑器改为全局统一圆角。标签填充用语义色 alpha.10、描边1；分组保留独立 alpha.09、20高配方（TaskClassificationBadges.swift:185–211），标签22高，保持原生图形。
5. 本轮不重建 Calendar、Day 日期条、运行页的变高行或右侧信息结构，不动服务调用、完成判断、runCommand、自然日推进、app 字段、flags、domain、模块配置。保留原型供对比。

## 验证与风险

- token tests：精确 RGB 舍入、语义角色区分、半径/字级/状态字重及侧栏 geometry。
- 运行现有根 UI tests；真实 runtime 通过开发服务命令合成任务，不载入生产/iCloud/凭证。
- NOONMARK_MYGO_BUILD_ROOT=/tmp/noonmark-visual-token-build，缓存和 PNG 全部外置；同一 ui.Render 生成原型 Day 2× 与 runtime Day 证据，实际查看像素。
- Linux 无 DISPLAY，不能宣称 macOS 系统字体、真实 hover/focus 或原生桌面视觉验收通过。保留原版当前中文文案；审计报告的 English target 并无用户要求依据，不作为本轮目标。后续应完整移植原版自带语言切换，不强制英文。
- 回滚仅撤回本任务新增 token/测试和对应视觉调用，不触及并行中的业务安全改动。

## 执行结果

已完成五个文件：本计划、visual_tokens.go、visual_tokens_test.go、main.go、runtime_ui.go。只修改共享色彩、导航组件视觉、Day 标题/任务字级及已有分类标记的布局，原有事务与自然日推进逻辑未动。

- `go test -p=1 . -count=1` 全部根 UI 测试通过（2.536s）；覆盖既有真实新增/排期/完成/撤销/重启/自然日/失败保护路径，以及新增 RGB/独立半径/字级状态测试。
- 新增 headless 导航 hover 进入/退出、选中不被 hover 盖色测试通过。首次实现曾将点击按钮放在文字前方，使文字成为命中目标；既有导航测试立即捕获，现已用按钮 children 正确归属文字和图标，并通过全部回归，没有弱化测试。
- `git diff --check` 与改动 Go 文件 gofmt 检查通过。
- 原型由 `go run -p=1 . -prototype -screen day -render-one /tmp/noonmark-visual-token-build/evidence` 生成；runtime 由真实开发服务和 UI 命令合成三项任务，完成其中一项。
- 外置 evidence：`noonmark-day-native.png`、`live-day-2x.png`、`live-day-compact-2x.png` 均2400×1536；`live-day.png`、`live-pool.png`、`live-completed.png` 均1200×768。全部实际查看像素。manifest.json 保留尺寸与 SHA256；ui-tests.log 与 render.log 保留命令结果。
- 目视确认：选中行圆角8、compact 图标居中、正常/选中导航字重分离、完成任务 regular/终态灰、标签轮廓及填色修正。源码值已准确表达，但字体轮廓仍不同于 macOS SF，不能宣称视觉全等。

## 保留差异

运行 Day 仍没有14天条、分组/子任务变高层级，所有路由仍复用65高行；Calendar 仍不是月格，右栏仍是通用会话统计。原版自带语言切换、SF Symbols 轮廓、原生字体与焦点/辅助功能、warmPaper/increasedContrast/reduceTransparency、真实桌面验收另需补齐。本轮没有虚构标签、子任务、分类或 AI/sync 能力；原型仍明确属于静态研究。

父级整体验证：2026-11-05 再跑 scripts/check（根 UI race 16.437s，taskapp 与三个领域模块 race、vet、gofmt 全通过）、scripts/cross-build（六目标完整 App 编译通过）、scripts/test-entrypoints（创建/重开/身份与复用保护通过）。检查日志仍在外置 build root；未运行真实桌面或 Swift oracle。
