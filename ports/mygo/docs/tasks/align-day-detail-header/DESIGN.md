# Day 详情顶部：源码几何对齐

任务 ID：align-day-detail-header。基线 cf9a77f8015f4dcf579e23c582fa53385091a7a6。

## 需求、现状、设计

用户要求 UI 尽量接近原版，包括圆角。本轮只调整已接通 Day 详情顶部：状态 Capsule、状态日期同排、标题层级、独立关闭按钮。已实际查看固定上游 Day 截图（内容为每日复盘，并非任务详情）与旧 Service 合成详情截图。旧版裸状态、独占一行的 ISO 日期、独立返回按钮不符固定源码。

沿用外置审阅设计 next-visual-page-design/DESIGN.md（SHA256 0da49012d33353474ae15d45a3170470f713f2f5f28e7d15a834d7ffb17709c0），但纠正日期假设：43e6eefaae3a04be89cab3e234d470111afd80ef 的 NoonmarkStore+PresentationUndo.swift 明确 displayDate 为 monthDay，AppDateTimeFormatter.swift 中文格式为 M月d日；weekday 为 zh_Hans_SG 的 EEE。因此本轮使用 M月d日 周X，不能加入年份。DayRow 不带 Date 字段，因此日期取同一 DayPage 的 Selected 投影，不取时钟，也不扩展领域模型。

标准状态胶囊按 WorkspaceUIPrimitives.swift:984–1011：11/500、5 DIP 点、5 DIP 间距、左右8上下2；Capsule 半径实际高的一半。颜色来自 Theme.swift。内部 cancelledDraft 或未知状态无可见样式，不崩溃、不映射 pending。状态与日期之间8 DIP是适配选择，Swift HStack 未给固定值。空间不足整段日期换行，保持字号。

Header 使用 DetailPrimitives.swift 的11/600/text3/LetterSpacing(.8)，正文14/600不动。独立 xmark 使用28×28目标、透明底、6圆角，复用原 titleTransition；不改变清选中前保存、失败、异步退役语义。保留旁边折叠按钮、间8 DIP。移除返回日统计整行。元数据前后14 DIP。

## 范围与风险

不改 Service、coordinator、持久化、自动保存、未接通功能、滑杆。composer 导航保留 object-keyed 草稿是既有语义，不擅改为提交或丢弃。无真实 macOS 同状态详情截图，只有源码配方与 Linux MyGo 离屏验证，不能称像素相同。SF Symbols/字体/continuous曲线及真实OS IME仍需对应平台验证。

## 验证

先症状红测，再隔离改动；geometry/颜色/日期/换行门禁、真实 Service 状态及长标题/多子任务/1×2×/最小窗口截图、关闭交互回归。最终源码执行 scripts/check、scripts/cross-build、CLI入口；所有数据与构建外置隔离。独立审阅后才由父线程整合，本候选不触发远端写或 CI。

## 保留的布局结构

本批仍保留移植端固定 header + 正文 Scroll 结构，并将 xmark 放在固定 header。未核定 Swift 祖先容器的完整滚动/纵向契约，因此仅称顶部组件配方与关闭语义对齐，不能称整个详情布局、滚动或绝对纵坐标等同。长可编辑正文仍沿用既有文本编辑器内滚动上限；没有把这种既有行为改为新字体或裁切策略。

## 冻结门禁的经审双路径增量

0031候选完整检查在旧历史审计冻结上判红：runtime_day.go视觉变更与旧hash不匹配。原FROZEN-PRODUCTION-SHA256.tsv（SHA256 13a9566458e76e1e9040d742362e8fbceecc544ef616a8b5030e12a4367435df）、Service overlay（df010edbbfa1eff8b167bb077890a06560c822de15e961a5fdef724a8731bca0）及command suffix保持原样。父审逐行核过视觉diff与old/new哈希后，批准附加FROZEN-PRODUCTION-OVERLAY-v1.tsv。初次单路径targeted仍在visual_tokens.go判红，随后一次性遍历全部冻结项确认只有这两文件不同，父审另批准第二个确切三元组。

该test-only overlay只允许runtime_day.go的68d1e5…→e761a9…与visual_tokens.go的214046…→5e29c9…两个确切三元组，在既有Service overlay之后套用。固定path、old及new，拒绝漏项/重复/额外路径/错误old/即使合法但未审new/坏格式/路径替代或穿越，以及缺失、变化、已经推进的baseline；复制map，不改输入。原逐文件hash循环与command suffix检查仍执行，不放宽旧历史边界。
