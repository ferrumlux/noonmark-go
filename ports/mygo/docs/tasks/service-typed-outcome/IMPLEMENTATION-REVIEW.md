# Service typed outcome 实现独立源码审查

日期：2026-11-06。**裁定：Accepted，仅限固定候选的源码／合同符合性审查；没有发现阻断项。** 本报告不替代同 HEAD 的运行验收、真正盲测、race 或聚合 check，也不批准 UI、RecoveryClose、恢复导出、v3 或产品发布。

## 固定来源与执行边界

- 候选：`71b81f01951b9a8cada87fa3fa4c2612f6c53b60`；基线：`eb924a14d395391c5625e61effddaed500ddb6b9`。工作树为空。
- production.patch SHA256：`200b26301db269f2ca96149fe01b0c13898a3b68e3fa0cd041719dd6ddac96e2`；production-sha256.tsv SHA256：`089a96f095b9590a48456b6741fce53249f6cc567818e07319b3ed91dd9fc3b6`。重新计算均相符，patch 与这两个 commit 的八文件 Git diff 完全相同，各实际文件与清单一致。
- 已读当前 ports/mygo/AGENTS.md、其引用的上游 AGENTS、Accepted outcome DESIGN 与独立设计 REVIEW；分别核对八份实际源码、全部生产 diff、新测试及旧测试差异。源码身份及逐文件 hash 见 SOURCE-VERIFICATION.json。
- 本轮只有静态阅读、Git diff 与 Python 字节/hash核验，`git diff --check` 通过。没有运行 Go 测试，不将阅读作者测试称为独立盲测；另一路运行验收须独立归档。只写本 sibling 审查目录；没有修改候选、正式仓、远端，也未读取 production、iCloud、Keychain 或调用 AI 网络。

## 逐路径裁定

### 1. 三维结果与 receipt

`outcome.go:10–96,178–217` 的 Result、WriteExtent、Session 独立且零值均无效。合法组合检查拒绝错误带 receipt、Uncertain/Writable、Confirmed 无 receipt 或无效 extent。Stage 与 HealthReason 为有限枚举，不持久保存错误或正文。

两个 typed 入口均从获取 s.mu 到执行领域命令、合并健康、发布已确认图及构造 receipt 全程持有同一锁。outcomeLocked 私有校验使用即将构造 receipt 的条件；两个外层构造器紧接着只在 `o.Err == nil` 时新分配 receipt，故当前源码不存在 error+receipt 或成功缺 receipt 的返回分支。内部协议兜底强制 ErrIncomplete、不给 receipt，并将任何非 None extent 保守映射为 ReservationPossible，不会把可能写入降为 None。它不是恢复操作，也不从错误名称猜写入阶段。

Manual receipt 的 percent 来自 normalize 后匹配 chain/trace 的真实 graph，并非 requested percent；缺匹配明确失败。nil→0 仍提交，clamp、历史 floor 与 100 不完成均沿用原领域实现。revision/hash 来自同锁下已确认 disk head。每次分配独立 receipt，成员均为值；后续写入或 Close 不修改历史 receipt。指针由调用者拥有，不是 Service 内部指针；调用者自行改自己的返回对象不改变 Service。

### 2. 实际 reserve／readback 时点与原错误

`store.go:273–365` 按实际分支记录观察：blocked、revision上限、encode、record容量、prefix、总容量、before-reserve 均未调用本次 Mkdir；容量和明确 before-reserve 失败不错误冻结健康。prefix 失败另标失效，保留其 ErrCorrupt/ErrConflict/ErrSession/ErrLimit 等首次错误。

只有紧接 Mkdir 前才设置 ReservationPossible；Mkdir 失败仍原 ErrConflict，不能据此假定没创建目录。Mkdir 成功后变 ReservedOrPartial，原 blocked=true 时点不变。payload、before-commit、marker、两次目录 sync 失败均保留原返回且 Uncertain/NeedsReopen。两次 sync 成功后才 WrittenUnacknowledged；after-persist 和最终 prefix readback 失败仍原 ErrIncomplete。成功后才增加 revision、更新 hash、清 blocked，返回 CommittedConfirmed。Service 只合并完整调用观察，没有在成功事务中途看到 blocked=true 就永久污染健康。

`verifyPrefix` 本体逐字未改，allowSuccessors 也未改；其只读包装将任意失败标记失效。提交前或 no-op 失败是 Rejected/None/NeedsReopen，并不声称先前残缺 revision 没有发生。提交后验证失败是 Uncertain/WrittenUnacknowledged，绝不 ack。没有新增 scan、补目录、repair、重试或提前发布。

### 3. 私有 I/O 替身没有放宽默认文件安全

新增 openWrite/readBack/syncDir/closeRoot 是本批故障可测性扩展，均为未导出 diskStore 字段；当前源码只在专项测试中赋值，没有配置项、公共 setter 或外部接口。

nil openWrite 路径仍精确 `root.OpenFile(name, O_WRONLY|O_CREATE|O_EXCL, 0600)`。writeExclusive 保持原 defer Close、Write、短写判定、Sync、显式 Close、ok 标记、readRegular 字节读回/相等核验顺序和所有 ErrIncomplete 映射。readBack 为 nil 时使用原 readRegular，包括原文件边界检查。nil syncDir 调原 syncDirectory(root,name)，仍先 revision 目录再根目录，未变同步平台限制。nil closeRoot 仍原 root.Close。没有让产品调用者绕过 O_EXCL、mode、sync 或 readback。

### 4. 所有现有 legacy mutator 同门

枚举当前全部 Service 方法并沿真实调用链核对：

- AdvanceTo→AdvanceToOutcome→reconcileDayLocked；AddPool/AddToday/AddOnDate/Schedule/SetCompleted→transact。
- 两个 manual legacy wrapper→同一次 SetManualProgressOutcome→setManualProgressLocked。
- RenameTaskTitle/SaveTaskTitleInput→editTitleLocked；SaveTraceTitleInputAcknowledged 在 trace guard 之前也先过门，其 error-only wrapper复用它。
- 十个 planned/actual subtask 操作→editSubtask；Defer/Continue→carryoverTrace；ReturnToPool 自身。

各根入口在原领域 guard 与 clock/random 之前调用 gateLocked。closed 优先 ErrSession；已知 NeedsReopen 或遗留 blocked 优先 ErrIncomplete。无关合法写入口也不能绕过。活 Service 的 raw disk commit/verify 调用仅剩 outcome.go 的两个封装；Create 的 d.commit 是尚未发布对象的明确豁免，Open 的 reconcile 已走新封装。没有发现其他接收者名或别名逃逸。

Health 只 RLock 复制现有事实、不扫描磁盘、不清健康；首次失效原因不会被后续普通错误覆盖。Close 在锁内先 closed=true、SessionClosed 再 root.Close，即使 Close 返回错误也不可复活，重复 Close 保持原 nil。Advance 的 invalid date 保留原 ErrInvalidInput 优先级，同时反映已 Closed/NeedsReopen；无额外磁盘扫描。

### 5. 健康行为保持与两阶段范围

原 manual 领域 guard／预检／重放／nextInstant／专用 audit／normalize／no-op 判断顺序保持；reconcile 的 frontier、settle、变化时钟及 today 发布时点保持。title/subtask/carryover/return 只替换共同门及磁盘调用包装；transact 只增加已知失效门。健康情况下 clock、random、fault、no-op 与写入调用数量没有新增；业务拒绝和候选失败不凭错误名自动冻结。

Advance receipt N 是当次确认事实；Manual 是独立锁与独立 attempt。不存在把后段 guard 或 uncertain 反向污染前段 receipt 的共享 attempt；允许两阶段之间其他命令，未伪造组合原子性。

## 测试修改与冻结门禁

旧断言调整只有三份行为测试：reconcile_clock 的同一失效对象第二次调用；title stale 的第二次调用；title/title_route 中先检测 blocked 再人为清 disk.blocked 的后续调用。首次原错误、closed错误、健康 no-op、clock及未发布断言仍在。没有删测试、泛化 skip、降低正常领域/audit规则或关闭 race/vet。

历史 FROZEN-PRODUCTION-SHA256.tsv 与 FROZEN-COMMAND-SUFFIX.sha256 逐字保持。新 overlay 精确七条 service/store/manual_progress/title/subtask/carryover/return 路径；old SHA 同时匹配旧 manifest 和 base 文件，new SHA 匹配候选实际字节。历史所有 manifest 条目在应用精确 overlay 后逐项核验通过，没有扩大忽略范围。新 outcome.go 单独 OUTCOME.sha256 匹配。旧 command suffix 实际字节 hash 仍为 `ac1d9c4a57753c20a3a906bbad64e961cd92bd115304528ccb7e2e8a8854b17d`。

overlay解析器拒绝漏项、重复、额外路径、旧hash不匹配、无效新hash、同hash；专项负例覆盖主要非法形状，历史冻结测试仍逐文件执行哈希，不是批准名单 skip。failure gates.tsv 仅追加本任务行，旧行未漂白。

已静态阅读的专项包含同错误不同阶段、所有旧写入口、prefix旧残缺revision、nil→0/no-op/clamp、两阶段交错、Close竞态、精确payload/marker Open/短写/Write/Sync/Close/readback故障、两目录sync、Close错误与容量/既有prefix上限区别。**这些是测试代码覆盖核对，不是本审查运行通过声明。** 当前 raw-call AST门禁按具体选择器形式识别，不是通用别名分析器；当前八文件已另行人工逐路径核对，其未来保证不应被夸大。

## 结论与后续验收界限

固定候选满足本批 Accepted 设计的源码边界，未发现应阻止进入同 HEAD 独立运行验收的问题。最终整体验收仍须结合真正盲测、专项故障、race、现有 taskapp/module 回归及聚合 check 的实际证据，分清 Linux执行与其他平台编译。WrittenUnacknowledged 不提供跨平台断电持久性保证。本结论不解除 manual-progress UI R5 的 Rejected 状态，不批准 RecoveryClose、原生草稿恢复或 v3。
