# GO-RUNTIME-014：剥除 continuation 关联后绕过通用审计

状态：已修复，仅限本案例实证的通用新增轨迹审计缺口。精确修复 commit：19e91ba035620b4d70231a21296a5fd2a78cc2af。原始症状与独立反例红转绿、Service受害路径、正式全量门禁均已通过；不等于全部产品、UI或生产系统已验收。
首次发现：2026-11-05，carryover Service 的授权边界负例探索。影响仅已实证的直接 CaptureEmptyHistory 入口；没有证据表明正常 Service/UI 可利用，也不是生产事故。

## 原始症状与归因

用真实 AddToday→AdvanceTo→core ContinueUnfinishedTrace 产生合法 continuation，去除新 trace 的 CarriedFromTraceID、ContinuationSeq 与 child carry links 后，ValidateTaskGraph 仍合法。旧 CaptureEmptyHistory 也接受该未在任务池的 chain 新 trace。无子项原始红断言为「generic audit accepted stripped continuation for non-pool source」，独立审查的已完成／未完成子项变体同样红。

原始日志：外置 noonmark-carryover-service-build-20261105/evidence/stripped-continuation-red.txt；独立子项红例：carryover-audit-build/evidence/child-stripped-baseline.log。红断言保留在 TestCarryoverStrippedContinuationCannotBypassGenericAudit；独立原测试保留在 TestIndependentStrippedChildContinuationRejected，没有更改预期或降低 validator 要求。

引入缺口的精确 commit：501fdfca858a6c437dca4875aaca4bd260627fbf，首次加入 CaptureEmptyHistory，未验证新增 trace 的原 chain 是否在池。Git author 与 committer 均为 ferrumlux <noxlimen@tutamail.com>，时间 2026-11-05 07:38:30 -0600。c984b7a013a0b8ba51faa5cbfc425868f4bcc637 的 return 防护仍保留此缺口。Git identity 不等于自然人，实际修改者未知。新 carryover core 不是该旧通用 API 缺口的引入提交。

## 机制、排除与修复

结构 validator 的通过只证明合法终态，不证明命令来源。新 trace carry link 黑名单只能拦明显延续，不能拦删除关联后的非池 scheduling。第一轮 eligibility-only 原型还重复使用同一 before pool membership，并未约束 current definition；独立审查进一步证明 old-pool 双 Schedule、历史 definition 和 empty-before 新 chain 绕行。三项红证据分别为 carryover-audit-build/evidence/batch-double-schedule-red.log、batch-old-definition.log、batch-new-chain-red.log。原型未验收，全部改为永久门禁。

最终修复对 generic 新 trace 批次重建有界 source 命令见证：实际 CreatePoolTask、ScheduleFromPool、MarkCompleted／UndoCompleted、SettleTaskDays 在同一个 exact-before kernel 重放；完整 canonical after 相等，没有字段剥除、任意新 chain 导入或 eligibility-only fallback。新旧 chain 都消费真实池资格。完整 planned-child 身份、内容与终态由 core 产生并进入全图比较。

这是合法命令见证，不是唯一真实日志或用户授权证明；Service 仍绑定真实 Today。明确支持多链 Create/Schedule/Complete/Undo/Settle，无法重建的专用编辑混合批次 fail-closed。具体兼容边界及排序见 ../../tasks/connect-trace-carryover-service/GENERIC-BATCH-DESIGN.md。首次全重放实现将同刻新 chain 按随机 ID 排序，独立合法 batch 判红；定位 FromDomain 保留数组插入顺序后，修正为稳定实际插入顺序，保留全图比较而不忽略顺序。

专用 carryover 始终按完整输入重放整图。没有放宽 codec、import、validator 或空 catalog 范围。无新 trace 的既有 generic 完成／结算审计规则保持；不宣称本批建立全部操作日志证明。

## 永久门禁、回滚与限制

gates.tsv 注册原始 fast gate、独立 child 变体与实际 Service lifecycle symptom gate，全部由 scripts/check 全包 race 运行。TestGenericScheduleAuditPreservesBatchesAndReturn 同时防止误拒多链 schedule+日结、真实 Return 后 Schedule。既有 Create+Schedule、planned child、多日结算门禁照常运行。

仅隔离 development/e2e 合成资料，无 production、iCloud、Keychain、AI 网络访问。回滚源码不得改写 session；旧 reader 不支持 deferred 的事实须明确告知。没有部署／灰度，监测为 typed errors 与自动化门禁。Swift、三平台原生、断电耐久性未验收。结案证据见下节。

## 结案证据

修复代码提交后独立回读其精确SHA。原始无子项及独立child剥关联红、旧pool重复、新chain绕行及旧definition红的断言均保留，最终作者／独立review／正式整合三层门禁通过。正式scripts/check退出0（root race202.020秒、taskapp54.285秒），包含注册的fast与实际Service生命周期／重开symptom gates。六目标编译与实际Linux隔离CLI创建重开／拒绝覆写通过，但没有非Linux实机、Swift或生产cutover证据。

正式代码与两份新增独立catalog／日期测试在测试开始前全部冻结；外置noonmark-carryover-service-integration-20261105保存事前哈希及check/cross/cli日志。只读审查报告SHA为5746bd0f8c5237ba007e62a45cee65a8887ffd27793dc3ab4ab858eeb150c4b3。此结案更新只涉及说明与gate索引备注，不变更实现或测试断言。GitHub待批，尚未推送此修复。
