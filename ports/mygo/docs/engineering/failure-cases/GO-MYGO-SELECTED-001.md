# GO-MYGO-SELECTED-001：Button.Selected 被暴露为 Checked

状态：已修复。正式实现 commit：0fcf95ecc4a134163d7df21215578daa555be89a；仅关闭本案例的 Selected 状态与适配层修复，不等于系统读屏实机验收。

- 继承缺陷：官方 MyGo v0.7.5-0.20261105040656-977ea57e0baf，ButtonBase.Selected(true).Background(Transparent) 的实际 RoleButton 状态 Checked=true、Selected=false。Swift 原版要求 isSelected；像素已对，但读屏语义不对。
- 责任边界：依赖框架既有缺陷，非 Noonmark 日期组件引入。现有 Git 是 shallow；977ea57e0baf78da62698ba7371420d192260466 作者 EGOIST <hi@egoist.dev> 是可验证快照提交，不是经验证的引入提交。真正引入 commit、author/committer、实际修改者均未知，不将 shallow blame 误作归因。
- 首次发现：2026-11-05 的日期组件独立审查；更早的用户实际受影响时间未知，没有读取或报告生产资料。
- 原红：独立 TestReviewCarryoverNativeAccessibilitySemantics，原日志保留在外置 noonmark-carryover-picker-review-20261105/final-focused.log；依赖审查的 original-red.log 另以精确四 patch 基线重新执行同一未修改测试。既有像素 TestReviewCarryoverHoverAndSelectedPixels 必须保持绿。
- 修正：patch005/006 独立 selected 状态及 macOS/Windows/Linux adapter，保留 Checked、按钮角色/actions/keyboard与旧选择角色；不伪装ListItem、不接受Checked fallback。
- 注册快速门禁：TestSelectedButtonIndependentAccessibility；依赖专项另有TestNoonmarkSelectedButtonIndependentState、TestNoonmarkSelectedOrderAndLegacyRoles。症状门禁：TestCarryoverPickerSelectedAccessibilityContract。聚合 scripts/check；专项 scripts/check-selected-access。
- 无真实OS/Swift验收；known TestDiffFrameAllocs 基线不在此修复范围。正式完整 scripts/check、原症状／像素、专项 Selected race、六目标编译及隔离 Linux CLI 已通过，记录见 docs/tasks/integrate-carryover-picker/VERIFY.md。

## 根因与受害路径

旧 Selected 与 Checked 共用状态，普通 Button 没有选择角色转换，所以视觉蓝色正确也不会产生真正 Selected 语义。修复追加独立 unset/false/true 状态和明确已指定位，不改变原有平台状态位值；显式 Checked/Mixed 与 Selected 的组合及调用顺序均须保持独立。选择类旧角色保留兼容别名，不以更改日期按钮角色、中文 Value 或 Tester 单独映射作为修复。

独立审查又捕获候选的三个通知缺口：Windows 移除 Selected 后仍须按上一帧 SelectionItem 能力发送清除；macOS 旧选择角色复用成普通 Button 须清除 AXSelected；普通 Button 的选中变化须进入显式 AXValueChanged，不能假定属性 setter 自动通知。候选均在正式整合前修正。Linux 真实 adapter stateList 的 selected/selectable/checked 组合已作无系统会话测试；这些源码及测试证据不替代实际读屏事件交付。

## 发行、回滚与教训

固定官方 archive、许可证、原四 patch 身份不变，新增两个 patch 及 NOTICE 全部进入已追踪源码。准备过程校验每个文件 before/after hash，并只使用外置只读依赖。正式整合须重跑整体检查、六目标编译和隔离 Linux CLI；新源码的候选包及离线重建另行验证，旧发布流水线成功不能代替本批。没有发版或远端写入。

此补丁不迁移用户数据。回滚 UI/依赖不得删除已有开发会话，也不得回滚此前读取 deferred 事实的能力。预防门禁同时验证通用按钮和真实日期组件，默认 scripts/check 经 check-mygo-native 的 Test(Noonmark|...) 执行框架专项；只测试像素或 Checked 不能证明选择语义。
