# Service三维回执与粘性健康：独立前置A

基线eb924a14d395391c5625e61effddaed500ddb6b9。设计SHA256 da4daf2284945b2c572a46a1cba9edbab5e97273d63bd4097c422330e64323ef；独审994d34e0da789bdb9b2d38565fb3ebfbc18e09905a36d842e1d1662ff72370de。本文是实现说明，不替代同head测试报告或独立审查。

## 源码所有权

- 新outcome.go：非零枚举，独立Result/WriteExtent/Session，有限FailureStage和HealthReason；每调用新分配receipt值，没有共享内部指针。Health为锁内值复制，不回推上次写入。
- store.go：原commit保留兼容wrapper；commitObserved按真实分支产生结构化观察。verifyPrefix原body与wire校验不变，verifyPrefixObserved统一标记纯读失败。旧fault位置和数量不变。
- service.go：AdvanceToOutcome及兼容入口，Service生命周期、统一写门、reconcile及transact。
- manual_progress.go：SetManualProgressOutcome及两个旧wrapper，同一次私有命令。原guard、预检重放、clock、audit、normalization、no-op、发布顺序不变。
- title/subtask/carryover/return：只更新共享门和磁盘封装调用；所有public mutator由真实调用门禁覆盖。

活Service只能由outcome.go中的两个封装调用disk typed primitives。Create/Open在对象尚未公开时的raw disk调用、store内部调用及私有测试是明确豁免；Open的reconcile仍经Service封装。不修改vNext/v3或历史helper。

内部可选I/O替身仅用于隔离测试，默认nil直接调用原os.Root/OpenFile/Write/Sync/Close/readRegular/syncDirectory。没有添加生产fault事件或改变四个既有fault回调的调用次数。测试分别令payload和marker的Open、短写、Write、Sync、Close、readback失败，及两个目录sync失败；失败后核对完整目录字节未被后续写门改动。关闭错误替身关闭实际资源再返回错误，验证Closed不可复活。

## 精确旧测试更新

Accepted合同明确要求已知失效后的后续调用返回ErrIncomplete：

1. reconcile_clock_test.go：同一损坏Service的第二次Advance改为ErrIncomplete；首次按原ErrCorrupt/ErrSession/ErrConflict，closed仍ErrSession，原clock/no-publication断言全部保留。
2. title_test.go TestTitleServiceStaleAndClosed：首次stale仍ErrConflict，第二次改ErrIncomplete。
3. title_test.go TestTitleServiceCanonicalEquivalentNoop、title_route_test.go TestTraceTitleStaleBlockedClosedCanonicalNoop：已检测blocked后手动清disk.blocked不得复活，后续期待ErrIncomplete；健康no-op、原图、关闭断言保留。

没有删测试、屏蔽race/vet或放宽业务错误。首轮完整taskapp的预期冲突原日志保留。

## 版本化冻结门禁

旧audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv逐字保留。新FROZEN-PRODUCTION-OVERLAY.tsv只准七个明确文件，严格oldSHA/newSHA/path；oldSHA必须匹配旧manifest，不许重复、额外路径或漏项。TestHistoryEditFrozenProduction逐文件验证版本化期待，其他旧条目和FROZEN-COMMAND-SUFFIX不变。负例验证非法overlay拒绝；新outcome.go由OUTCOME.sha256单独冻结。测试不是简单忽略批准改动文件。

## 验证边界

maxRevisions hard-cap测试只断言未写且不错误污染健康，不宣称换较小请求可突破硬上限。现有prefix条目超限同样ErrLimit却必须NeedsReopen。测试使用真实Service领域fixture，再对隔离磁盘施加故障；硬上限分支测试明确使用内部revision替身而非伪造1024次已提交事实。

没有生产、home、iCloud、Keychain、live AI、远端写/CI。WrittenUnacknowledged只描述本平台原sync/readback阶段，不承诺断电durability。UI R5仍Rejected，RecoveryClose、原生草稿导出、恢复查看器和滑杆均不在本批。

## 共同基线整合验收

在正式eb924a14上整合固定71b81f0八个production文件与原样独立测试（SHA e193b62f5728cb5fe965bd6ba34574cf87804b90be42c1c76187a6caa950e967），另保存独立源码审查和验收报告。固定577文件完成全scripts/check，root/UI race524.419秒、taskapp race259.443秒，四模块race/vet、Unicode、依赖准备、MyGo native全通过。six-target编译、host构建及Linux development day/pool渲染、创建/重开/拒绝复用等CLI守卫通过；源SHA/mode前后完全一致。

整合定向先过作者专项及冻结gate race6.261秒，随后用原文件解析出的十个精确名字单独运行独立race3.720秒，登记race1.507秒。不把宽名字筛选当精确独测。构建顺序复用同执行者已结束的外置cache，因此不是冷缓存复现。完成后仅回填本文和故障记录，产品/测试源码未变，文档相关登记另行复跑。

当前这份结果是本地共同源验收，正式同步commit与对应hosted CI尚待核对。六目标仅编译、headless不是桌面/IME、故障替身不是物理掉电证明；未启用RecoveryClose、手动滑杆或正式发行。
