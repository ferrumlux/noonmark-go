# GO-RUNTIME-013：合法极值时钟使无变化重开失败

状态：已修复（本地隔离研发验证；未推送、未发行）。修复 commit：e056d25c84f32ac46245f1d1d44f06bbcd4c6ceb。原始症状由红转绿，正式副本全量门禁、六目标编译与Linux隔离入口已通过；不代表Swift、桌面实机或滑块性能验收。
首次发现：2026-11-05，手动进度独立审查；影响既有开发 Service.Open 与日结入口，不是生产事件。

## 症状、归因与证据

真实 core.SetManualProgress 把内容时间设为有限 math.MaxFloat64，经完整重放审计、normalizeSnapshot、开发磁盘 commit 后产生合法持久图。同一天 Open 没有任何结算变化，却返回 ErrInvalidClock。独立审查连续三次红；接手者再次原样复跑也红于原断言第34行。

现有 Open → reconcileDayLocked → nextInstant 调用形式来自精确 commit：
501fdfca858a6c437dca4875aaca4bd260627fbf。
该提交 Git author 与 committer 均为 ferrumlux <noxlimen@tutamail.com>，时间均为 2026-11-05 07:38:30 -0600。
最早引入本故障的 commit 是 4cbd56883b8a6065c82902923b66ecc0741a2cef：其 Open 已先调用 nextInstant，再执行 SettleTaskDays 及无变化比较。501fdfca 保留该错误顺序并移入 reconcileDayLocked。4cbd5688 的 author／committer 同一 Git identity，时间均为 2026-11-05 07:02:54 -0600。
Git identity 不证明自然人实际操作，实际修改者未知，现有证据不能归因自然人。

原始红例 exhausted_frontier_reopen_review_test.go 的 SHA256：
e4b9d91965e362d0cc18c4fb649ec12b6cf5b5d00b1ba042cd56be1db5dea9df。
文件与断言原样进入永久测试，未改期望、阈值或合法 fixture。
外置证据目录 noonmark-manual-progress-build-20261105/reopen-safety 保存 red.log、首次矩阵日志及最终回归日志；独立原证据仍保留。

## 破坏机制与排除假设

NextMutationInstant 必须严格晚于全图已持久化 frontier；MaxFloat64 后一个值为 Inf，因此真实变化必须拒绝。但无变化的重开不需要新时钟。旧实现把分配时钟置于结算判定前，使合法可读的会话无法重开。不是 JSON 精度丢失、非法图、manual clamp 或进度投影缺陷。没有否定已通过的 validator／codec 契约，也不使用普通 wall clock 无法产生极值来掩盖合法输入。

## 根本修复

全图 persistedClocks 由 nextInstant 与只读 persistedFrontier 共享。私有结算预检使用已有最大有限时钟，执行实际 SettleTaskDays、CaptureEmptyHistory 与 normalizeSnapshot。无变化不采样 wall clock、不推进 revision，仍检查 closed、blocked 与完整 prefix。空图用合法零值做无写入预检。真正变化须重新分配严格单调时钟，从原图重新结算、审计、规范化并一次提交。预检结果不发布。

AdvanceTo 不再按 today 相等直接跳过，因为相同日期不代表历史已经结算；同时堵住旧快捷返回绕过 blocked 与 prefix 验证的路径。任何失败保留原任务、分类审计与 today，after-persist 的不确定成功仍由重开恢复真实落盘状态。

## 永久门禁与回滚

gates.tsv 绑定原始症状测试及 TestReconcileClock 四组矩阵，全部进入 scripts/check 的全包 race 门禁。矩阵覆盖常规／极值同日重开、跨日真实变化／极值拒绝、相同 today 尚有待结历史、空图与未来图 no-op、倒退 wall clock、closed／blocked、旧 payload／seal／身份损坏、跨 handle 冲突、四阶段磁盘失败及重开。既有跨日审计测试继续运行。
首次新增矩阵误把 openStore 的扫描失败也预期为 ErrCorrupt；源码与运行显示既有 Open 返回 ErrIncomplete，已只纠正新增断言，保留 live verifyPrefix 的 ErrCorrupt 断言与原红例不变。

本批不发行，不改 wire/schema，不访问生产资料。回滚时撤回新增功能及 service 安全变更，不改写会话；旧代码仍可能不能打开极值会话，须明确这项回滚风险。没有灰度部署或线上监控；仅隔离故障注入与永久门禁。六平台编译不等于 macOS/Windows 实机验证，未运行 Swift oracle 或断电测试。

教训：先问操作是否产生事实，再分配不可逆的逻辑时间；无变化仍必须验证持久化前缀与句柄状态，不能靠日期相等或放宽时钟规则逃避问题。

整合闭环：正式副本 scripts/check 退出0（主模块race140.881秒、taskapp34.092秒），six-target cross-build与Linux create/reopen/reuse/identity保护入口均退出0。原红断言未变，另纳入独立倒数第二有限时钟结算及未完成successor拒绝两项永久回归。详见 connect-manual-progress/VERIFY.md。
