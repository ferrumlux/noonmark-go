归档说明：以下为本批设计或隔离验证的阶段记录，其中待实施／待整合措辞指当时状态。原文 SHA-256：`c869a5890f6b34f4997b71950a9e3b2dd2e6ab6e4009ed41843f09a0658b07e9`。归档仅调整协作措辞与任务标识；正式整合结果另见本目录 README。

# Typed 日结束与完成历史审计：独立验收

日期：2026-11-06。结论：**Accepted，未发现阻断。** 冻结的两个 production helper 通过独立源码审查、来源正负例和完整 taskapp test / race / vet。此结论仅允许进入整合阶段精确整合门禁，不是 Service/writer、v3、Swift 互通或全跨平台验收。

## 1. 精确对象与独立性

- 基线：`noonmark-go` @ `03a18cca51766e1d6b501e5e015ce96d0be5a7c7`，检查时正式树 clean。
- 作者冻结 patch：`/workspace/shared/noonmark-history-command-vnext-20261106/history-command-vnext.patch`，实读 SHA-256 `9c2c430dddc484990e6625ceaefc6c504671bf0c795de2384786bcd8ebbec601`。
- 作者 FREEZE.json SHA-256：`56ed1237c60f99300a8b93266704671e75b3ac781e155857257d5b9855476bf5`。
- `history_command_vnext.go`：`cf81215a29b288712592a1c768647e74825eec2eed363b0ad8f3d4bb90210118`。
- `history_command_vnext_budget.go`：`035c27d97ef66e97f7d0cbb1d534368a327ff6c36411e144f9c0320e851b4b38`。
- 已核对 accepted R2 设计、独立设计准入、upstream `43e6eef` 的 AGENTS/相关 CONTEXT/ADR-0022/0037、Go AGENTS 与实际 Go/Swift 来源。完整前置来源矩阵见 `PLAN.md`。

独立源码副本：`/workspace/scratch/586e77bd59a6/noonmark-mygo-work/noonmark-go-history-command-independent`。从正式 commit 的 archive 建立，只导入作者两份 production 文件，**没有读取或导入作者三个新测试文件**。旧 486 个 tracked 文件逐一比 SHA，全部相同；未修改正式树、作者冻结物或 accepted r1。`evidence/source-verification.json` 与 `baseline-files.sha256` 保留证据。

自己的测试先根据冻结基线的 core/Swift 合同设计，随后针对稳定 API 编写。来源 graph 经 core 命令产生；分类期待值用独立 schema 对象及明确事件顺序投影，不调用待测 command/capture helper、append builder、作者 fixture 或输出生成 oracle。纯表示/预算/clock 反例与来源正例分开，不冒称 Service 可达性。

## 2. 静态审查结论

1. `history_command_vnext.go:93–108,150–163`：先检查 sealed prior，两个 caller graph 与全部输入 IDs 通过固定 metadata-only preflight 后，才调用 bind。每个失败分支立即返回零 result。配套 AST 门禁已实跑，不只靠错误类型推断顺序。
2. `history_command_vnext_budget.go:41–214`：checked uint64 加乘、五顶层总成本、每个 nested slice 预扣、全部 optional 成本与 34 处逻辑字符串叶完整。只读 len/nil/header；没有内容扫描、clone、索引、reflection 或 caller-sized allocation。共享叶片逐逻辑出现计费；before/after 各自预算。EventIDs 数量只由 sealed prior trace count 与固定上界限制。
3. `history_command_vnext.go:235–275`：prior 副本 ToDomain 得到自有 graph，完整 before 绑定并再次 exact round-trip；caller after 与单次 core replay 以原 Supplement 重组成整份 snapshot 比较。小写 task rawValue 不会静默规范化通过；旧分类 key/event/traceID 的小写可原样保留。
4. `:122,194–197,277–304`：日结束重放一次 SettleTaskDays；complete/undo 在封闭 switch 中只执行所选动作一次。capture 只由新锁及 before P/C 派生，按 date/TraceID 排序。open U/A/R/D silent lock 不重复 capture；X-only 不锁。
5. `:310–339`：固定 append builder → 完整 after inspector → 旧前缀 → 非分类 supplement 冻结及精确数量。两入口再核对命令固定冻结段；没有 callback、flags、exemptions 或提前发 seal。完整 inspector、prefix 和 exact replay 没有互相替代。
6. result 只保存私有封闭 kind、scalar input、owned immutable endpoints 与派生 changed；snapshot 查询深复制，零值拒绝。没有接入 Service/store/reader/UI/v2/wire。正式旧文件不变的逐文件证据比新增文件中的注释更强。

## 3. 独立运行矩阵

两份自有测试共 14 组、94 个命名子项，另有表内循环与并发循环：

- P→U 与 C 新锁；open U/A/R/D 零 capture但 changed；X-only 真 no-op与 mixed X；已锁结果重新审计 no-op。
- 多日期、反向数组、多 chain 的固定 date/TraceID→EventID/revision 映射；少/多、非法、同批重复和大小写冲突 IDs。
- P/latest A→C/latest A→P/latest A 多轮；分类完全保留，随后 C 锁日保留真实 completedAt。旧分类小写 key/event/traceID 的 complete/undo/append 原字节保持；小写 task rawValue reader 可读但命令 exact binding 拒绝。
- before/after 上合法但无关的 chain clock/note、trace note tombstone、definition title/clock、progress、priority、day review、description、数组换序、额外对象；两入口 wholegraph 拒绝。child 变化也不通过。
- owner/target/action/Today/Now 欺骗、错误重复状态、pending/unfinished child guards、旧 owning chain clock。负有限、小数、子正规、±0、MaxFloat、NaN/Inf、日期端点与非法日期；无关较新 chain clock不被误当纯 helper 全局 frontier。
- core 精确 replay 能成功的 A@10 → silent lock@5，最终完整 profile 拒绝；builder 旧 ID 冲突返回零结果。两个各自 reader-valid endpoint 的旧 event identity/time 变化由 prefix 拒绝；缺 required U capture 由完整 inspector 拒绝。明确这些分类反例不是 API after 参数中的字段。
- 成功结果与 caller graph/IDs、返回 snapshot 无可变别名；失败 prior/before/after/IDs 不变和零 result；共享 immutable prior 并发 complete及日结束调用、并发取副本在完整 race 下通过。
- 独立完整 typed model manifest；34 处 string/alias/optional/nested 逐项超 B；23 处 optional 成本增量；实际 canonical task forest 节点/raw bytes 对账；五顶层、嵌套 notes/planned 及总和超 N；共享 slice/string/pointer 逻辑重复计费。
- IDs count/length 在领域无效 before/after 前先拒绝；两个 graph 分别触发 typed budget phase。checked add/product 以 big.Int 独立 oracle 覆盖极值，无巨大分配和溢出后比较。
- raw bytes=B gate通过、B+1 gate拒绝；**完整 canonical envelope 恰好16,777,216 bytes 可 inspector认证和命令 no-op成功**，两个 graph 不合并成一份预算；再增加1个编码byte仍可过 metadata gate但最终 inspector拒绝。没有用 OOM、timeout 或 RSS 当断言。
- AST 前置 gate/立即 return、完整 bind/replay/match/finish 调用计数、append→inspector→prefix 固定顺序、预算调用白名单；旧 reader仍拒绝 A、多事件/open U，且全 taskapp 旧行为回归通过。

这些测试未出现失败运行，无需要保存的候选最小 red，也没有修改 production 使测试变绿。初轮、扩展、最终 focused 日志全部保留，最后 focused 包耗时4.832s。

## 4. 实跑命令与结果

环境：Go1.27.3 linux/amd64，通过候选 `scripts/environment`；外置 BUILD/GOTMPDIR/GOWORK，GOCACHE=`/workspace/shared/noonmark-go-build-cache-20261105`，GOPATH=`/tmp/noonmark-gopath`，GOPROXY/GOSUMDB=off，GOMAXPROCS=2，GOGC=20，GOMEMLIMIT=700MiB，-p=1。入口为本目录 `run.sh`，不复制或清理 toolchain/cache。

| 实跑 | 结果 | 包耗时 / 命令墙钟 |
| --- | --- | --- |
| `go test -p=1 ./internal/taskapp -run '^TestIC' -count=1 -v` | exit 0 | 4.832s；最终 focused |
| `go test -p=1 ./internal/taskapp -count=1` | exit 0 | 27.571s / 28.632s |
| `go test -p=1 -race ./internal/taskapp -count=1` | exit 0，无 race 报告 | 164.255s / 172.092s |
| `go vet -p=1 ./internal/taskapp` | exit 0 | 1.586s |

完整 taskapp race **没有 `-run` 子集过滤**。作者仅 history 子集的 race 结果未用来替代独立全包 race。完整时间、命令、退出码见 `evidence/runs.tsv`；三个 focused及 full/race/vet 原始日志均在 `evidence/`。

## 5. 完整交付与边界

独立测试补丁：`independent-tests.patch`，SHA-256 `726277f0ddc4843f770fea758334efcad337f4a62f996ba0bc254977f1453610`。只含两份新增测试，可由整合阶段按项目策略保留；不含生产修正。

- `history_command_independent_test.go` SHA-256：`a1f381662a39fe35b3e3f5751eba6040b81516d3da491c69a641eb3ab3d4d24d`。
- `history_command_budget_independent_test.go` SHA-256：`1d50a5d00c0daf25a29a96345551e2b4ae216ed02d6373f0e88f8b74c53c6ea9`。
- `SHA256SUMS` 覆盖报告、计划、补丁、入口和证据。正式树在独立验证结束检查仍为基线 clean。

没有运行 Swift oracle、非 Linux 实机、整项目 scripts/check、cross/build/CLI、UI、live AI、真实会话或新 Service/store writer fault/reopen/Today 路径。旧 taskapp 中既有 store/Service 测试被全包运行，但不能据此称新 helper 已接线。没有 production/home/iCloud/Keychain/GitHub 操作，没有正式 commit 或任何远端写。

已知来源边界保持：Go complete/undo 对早于 owner clock 的 now 拒绝，Swift touchChain 的 max 语义仍仅静态对照；Go 排序不代表 Swift Set 顺序字节对等。metadata 预算不是精确 RSS/耗时担保；AST 门禁配合审码，不宣称任意 Go 程序的形式验证；caller 并发写输入不在合同内。

后续仅建议：整合阶段在精确整合树运行项目要求的聚合、cross/build/CLI 门禁，继续保持未接线。Abandon/Reactivate writer、完整 persisted frontier、新 development v3、磁盘与 UI 是另阶段；不能凭本次 accepted 直接扩权。
