# GO-UI-CARRYOVER-PENDING-001：保存中日期 sheet 的关闭语义

状态：已修复（仅本地已验收范围：保存中文案与可访问语义；关闭不撤销已准入写入）。实现提交 `fbfa97dd810121d3fae268c39cc621882efbaba5`；无新hosted CI、实机或发行验收。

首次发现：2026-11-06。Go + MyGo 异步 carryover 准入后所有目标控制已禁用，但仍显示选择问题、选择范围提示、「取消」与原确认动作；日期数字又被不可选色及框架 Disabled 双重变淡。用户容易把仅关闭展示误解成撤销已经准入的保存，目标也不易辨认。

引入提交：961e688dcf0077c7976a61e6f5e1279ef6254960（正式整合普通 Day 异步 carryover）。Git author／committer 均为 Codex <codex@openai.com>，提交时间为 2026-11-06 01:17:55 UTC；该 Git 身份不等于已验证的自然人身份。本批最初从未提交 R7 候选复现，正式整合时补录后来已存在的引入提交，未追改原冻结候选。Swift 43e6eef 同步原版没有本异步阶段，本案不归因于 Swift 或生产。输入冻结为 R7 35 文件 manifest SHA-256 7576deb264386cc41de6e7a8122708a374267ff006eee895ccf2ad56464d7324。

复现与排除：真实 MyGo Tester 经原生 Day 菜单确认，受控 writer 等待时录得旧文案及淡色日期；新增组件状态/可访问/selected 像素门禁均红，真实按钮路径无法找到「关闭日期对话框」，Escape 路径仍能安全保存。源码和既有测试证明写入合同本身已经正确，因此本案不改 writer、guard、取消行为、navigation fence 或磁盘层。

根因修订：只在 pending 原位置替换标题为「正在保存延期…／正在保存延续…」、提示为「关闭不会撤销已提交的操作。」、按钮为「关闭／保存中」，Description 如实说明关闭仅撤展示及等待结果。所有日期保持 Disabled=true、Invalid=false；pending 的 selected 与合法日期使用公开 DimDisabled(false)，保留选中白字和原文字 token，不把等待当成无效日期。未提交状态和布局逐字节截图对照不变。

自动门禁：fast 为 TestCarryoverPendingCopySemanticsAndGeometry 与 TestCarryoverPendingSelectedDateRemainsReadable；symptom 为 TestCarryoverPendingCloseAndEscapePersistOnceWithoutStealingNewSheet。真实菜单路径包含延期／延续 × 关闭按钮／Escape × 原展示／真实菜单重开新 sheet 八组合，重复 Enter 不重复提交，关闭保留 source lease，旧完成不抢导航或关新 sheet，最终 graph 精确回读相等。既有 pending 的全部安全断言保留，只明确迁移三个 Label 期望。

证据：本批外置 BUILD/evidence 中 pending-copy-red.log（exit1）、pending-copy-green.log（exit0）与 red/green 2× PNG。原红测试 SHA 已冻结，红绿期间同字节。完整 focused/race 结果另见本批 VERIFY；尚不声称总体 gate 或 OS 验收。

回滚／发行：仅撤 pending 的 UI 表达，不修改已确认资料。无提交、远端写入或 release；完成状态仍待整合阶段组合批次聚合门禁与修复 commit。教训：引入异步阶段时，关闭 presentation 与撤销 operation 必须在文字、可访问说明及真实中断路径上明确区分；disabled 不应把已提交目标隐藏。

## 正式闭环

实现提交 `fbfa97dd810121d3fae268c39cc621882efbaba5`。在正式组合中，完整 scripts/check、六目标编译、主机构建及实际隔离Linux CLI全部退出0；精确门禁与证据见对应任务 VERIFY.md。所有业务源码及原测试断言在验证期间保持不变。本文先前“待整合/未提交”等描述是候选阶段证据，不代表此处最新状态。修复没有修改生产资料、重试待批Git操作或正式发布。
