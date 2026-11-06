# 普通退池、延期与延续的未接线历史审计

本批仅新增 package-private pure typed Return（当前 R／未来 X）、Defer、Continue 入口。没有接入 Service、store、UI、v2 或 v3；没有改 core、reader、profile 或 allocator。已有消费者仍沿用旧路径，不能把本批 proof 当会话提交权限、真实历史可达性或平台对等证据。

## 契约

- 首先验证 prior seal，再分别预算 before、after 和原始 input，然后才 bind、复制、验证或重放。map 数量先限制于 sealed prior 的 subtasks 长度及 MaxNodes/2；每个 key/value 和未用 scalar 都计 raw bytes。调用期间 caller 不可并发改 graph、map 或共享 leaves。
- 保留 core guard/error 顺序，先精确重放、完整 after 比较，再检查 capture 数量。R/D 各追加一个固定 source event；X/Continue 零 capture 仍经过完整 inspector、successor 和固定 fence。future X 带旧流不能靠删流或降级 inspector 通过。
- 独立 fence 从 prior 和 typed request 构造完整预期，冻结不允许改的所有字段、数组位置、optional presence 与 Instant bits。只复制 source open children，保 lineage/carry-from；Continue 不改旧 source。
- 成功 proof 是三种封闭 kind/detail，持有 owned input map 及 sealed endpoints。snapshotCopy 深复制；失败返回零结果。

## 验证入口和限制

环境要求 Go 1.27.3，先设置独立且已存在的外置 NOONMARK_MYGO_BUILD_ROOT，按仓库构建环境设置 GOCACHE、GOMODCACHE、PATH。不得使用系统旧 Go，不生成仓库内 BUILD。

定向门禁：

- `go test ./internal/taskapp -run 'TestHistoryRemaining|TestRemainingDirect|TestHistoryCommandBudgetASTOrderGate' -count=1`
- `go test ./internal/taskapp -count=1`
- `go test -race ./internal/taskapp -count=1`
- `go vet ./internal/taskapp`

首批测试在实现前真实编译失败，新增接口未定义。实现者测试和独立验收是两回事，不能称为盲写验收。完整外置命令、退出码、原始失败、最终 SHA 与未覆盖项以本批交付证据为准；此 README 不预先宣称未运行门禁通过。

原有生产文件除 history_command_vnext.go 的新增 kind、detail 字段与 valid 闭集校验外逐文件冻结。该文件其他旧函数 body 冻结。独立验收发现旧 kind 接受新 detail 后，按独立Accepted澄清为旧 kind 加入三新detail均nil条件；旧真实可生成proof行为不变，不再声称valid旧分支逐字不变。core 与旧 Service/writer 没有因本批改变。新增接口 AST 进入既有前四关门禁，另检查预算 metadata、唯一 core 调用及封闭 proof 构造点。

尚未涵盖 create/schedule、运行时 head/session 与全局 clock、v3 reader/writer、故障重开／双 handle、Swift oracle、非 Linux 实机或 UI 行为。真实proof闭集缺陷记录于 GO-HISTORY-REMAINING-001.md，并已登记fast/symptom门禁；fixture错误另记外置记录。

## 正式共同基线整合验证

在 ed200ac5d4e269fb23c884b3f19d817f928e8d87 上整合修订候选32befa5的精确四production、作者测试和两份独立测试。原独立七测试存档SHA6036dba28094dd0d8d7622e42b0de2d5d21646a1a2575ae9371a33eefb136610保留；整合时go vet抓到三处无字段名LocalDate字面量，仅改为Year/Month/Day具名字段，所有值和断言不变，整合版SHA31c5845fb478ce9de92285266ce6a5815fd11b02ffb06f29f25fa769a4ee709a。原测试来源未重写，不再声称整合文件逐字不变，另一个闭集矩阵独立文件SHAa813ecabfbe33db4d21e69eca18baa01de1719ca5896fe1cff7c716c468d6da1。原2d76afb失败版本未混入本批。

最初定向回归3.471秒退出0，首次check在root/UI race528.958秒、taskapp race245.906秒通过后被go vet的三处无字段名字面量拒绝（exit1）。原红日志已保留，不能把首次整体称通过。完成上述仅测试字面量修正后，重新实际执行vet、独立focused和完整聚合；同一固定564文件的 scripts/check（四模块race与vet、Unicode、依赖与MyGo native门禁）、scripts/cross-build、scripts/build、scripts/test-entrypoints 全部退出0，文件SHA/mode前后核对一致。使用已核官方Go1.27.3与离线依赖，外置隔离构建，顺序复用同执行者上一批已结束的编译cache，因此不是冷缓存复现；CI会另按新head核验。

六目标只有编译，Linux CLI只有MyGo headless day/pool与development创建/重开/复用守卫，不是六OS桌面、IME或发行验收。此后只更新故障状态与hosted证据文档；没有更改production或测试，相关登记门禁已实际race重跑退出0（1.494秒），独立行为／闭集／原症状race定向重跑退出0（6.228秒），go vet再次退出0。正式修复commit eb924a14d395391c5625e61effddaed500ddb6b9 已同步；同head MyGo run42198336290及实际artifact核验通过，见HOSTED-CI-42198336290.json。此后没有把这份前批证据借给新的Service变更。
