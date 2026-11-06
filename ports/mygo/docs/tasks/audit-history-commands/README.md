# 未接线日结束与完成命令精确历史审计

task-id: audit-history-commands

日期：2026-11-06。级别：P。状态：设计 R2、独立复核及正式整合门禁均已通过；仍未接线。

## 需求、现状与设计

基线为 `03a18cca51766e1d6b501e5e015ce96d0be5a7c7`。上游只读来源为 `43e6eefaae3a04be89cab3e234d470111afd80ef`。沿用 AGENTS、领域词汇及 ADR-0022/0037；本轮授权的新源码副本取代根 AGENTS 针对另一台 Linux 主机的固定源码路径规则。除移植 README 加入本批入口外，原有 486 个 tracked 文件中的其余文件逐字节及模式保持不变；旧三个 r1 实现及三个测试、Service/store/UI/core/v2/wire/codec callbacks 均不改。

已复核设计 R2 SHA-256：`d8d0391913f3aea0f7bb5dec391654624b0fe72785abf23797b514d9b21a3f2d`。Accepted R2 报告 SHA-256：`aedb79b4fb0c5c96394b005991a7f54de41e21835b972e1a77c1cbea396fd124`。设计和审查原文保留在本轮外置证据目录，由外置证据归档保留；本文件不是替换版设计。

本轮只增加 package-private 的两个纯审计入口及封闭结果：

- `auditSettlementHistoryVNext`：sealed prior、整个 before/after、Today、Now、EventIDs。
- `auditCompletionHistoryVNext`：同上，但输入是 complete/undo、ChainID、TraceID、Today、Now，没有 caller captures。
- `auditedHistoryCommand`：内部绑定 fixed profile、closed kind、scalar input、prior/final proof、派生 changed；仅返回深复制 snapshot 和 changed 查询。零值拒绝。

## 实施合同

1. 先检查 prior seal；两个 caller graph 分别执行 metadata-only 节点及 raw string bytes gate，再检查全部输入 IDs。全部通过前不 Validate、FromDomain、Clone、建索引或扫描内容。五顶层 array 起算5，使用 checked uint64 加乘；全部可选字段、嵌套 slices 和 string aliases 明确列出，逻辑共享重复计费。生产不使用 reflection、callback 或 caller limits。
2. 从 prior 解码自有 graph，以原 Supplement 完整绑定 caller before 及 exact round-trip。只在自有 graph 重放指定 Go core 命令一次，再以原 Supplement 比较整个 caller after。
3. 日结束只从本次新锁及 before P/C 派生 U/C captures。已有 U/A/R/D 的 silent lock 零 captures但 changed=true；X-only及空日是真 no-op。按 date、规范 TraceID 排序，EventIDs逐项绑定。完成与撤销完成零 captures，原分类整份保留。
4. 固定执行 accepted append builder → 完整 after inspector → prefix → 补充固定冻结段与数量检查。三个证明层互不替代。
5. 成功才产生封闭结果，changed来自完整 prior/final canonical 比较。失败不改 caller graph、IDs或sealed prior；结果不暴露可变 proof。

不能将 helper 解释为写权限、合法会话绑定、全历史可达性或 Service 时钟。它不读墙钟、不分配 UUID/nextUp；数值相等及 finite MaxFloat64 在纯 core/profile 允许时可成功。未来 Service 的完整 persisted frontier、严格前进 Now、写锁、reservation、一次 commit、重启和 Today 发布仍是独立阶段。

Go complete/undo 对 `Now < owningChain.UpdatedAt` 的拒绝保持不变。上游 Swift 的 `max(old, now)` 差异尚无 Swift 实证，本轮不修 core，不称全域跨引擎对等。Go 的确定性 capture 排序也不是 Swift Set 顺序的字节等价承诺。

## 自动化验证映射

新增测试均为合成资料。正常来源先用现有 core 命令生成 graph，预期分类事件由测试显式投影，不使用待测 capture selector、builder或结果生成 oracle。针对表示/预算边界的手工 graph/AST 均单独标注，不冒称 Service 来源。

| 测试组 | 主要检查 |
| --- | --- |
| SettlementSources / CompletionSources | P/U/C/A/R/D/X、silent lock、既有锁、old A、多轮完成与撤销完成 |
| DeterministicMultiCapture | 反向数组、多日期、多链、旧/新流、固定 ID/revision/status/Now，缺/多/旧/重复大小写/非法 IDs |
| WholeGraphMaliciousDeltas / SettlementExactDelta | 所有未触碰链、定义版本、附言与 tombstone、child、priority/progress、对象增删/换序、旧锁及伪造 before/after |
| EligibilityAndProfileFailure | owner/action/target/日期/状态/children；现有 Go chain-clock 拒绝；core 成功但完整 profile 拒绝的 A@10 → lock@5 |
| ClocksDatesAndNoop / SignedZero | 合法端点日期、无效 Today、负有限/小数/子正规/相等/MaxFloat、NaN/Inf；零的位型及真实 completedAt |
| MixedXAndUnrelatedFrontier | mixed X+companion、空日、无关更晚 chain clock，不偷加全局严格递增 |
| IDRepresentationAndHistoricalPrefixLayers | 小写 task rawValue 精确绑定拒绝；旧分类小写 key/event/traceID 保留；两端reader-valid的旧ID/clock/删A篡改由prefix拒绝 |
| ImmutableAliasesAndConcurrentProof | 零proof/result、caller/return-copy别名、共享只读prior并发命令与copy；不承诺caller并发写安全 |
| BudgetModelManifest / NodesAndEveryOptional | 完整字段/类型清单防漂移，7种record成本、23处optional增量，独立wire树节点对账 |
| BudgetAllStringLeaves / RawEntryPrecedence | 全34处string叶逐项超B、102字节总量对账、无效UTF-8不先扫描、两个graph/IDs前置阶段、失败不变 |
| BudgetArithmetic / LogicalSharing / NearEnvelope | checked溢出、nested/top总和、共享逻辑叶、raw B/B+1、nodes=N及超限、接近16MiB的合法UTF-8输入、最终envelope拒绝 |
| BudgetASTOrderGate | 两入口显式preflight立即return且先于bind；metadata调用白名单/索引访问；append→inspector→prefix固定次序 |
| ScopeAndOldReaderStayClosed | 非支持supplement/scope无proof、旧reader和旧codec仍拒绝A/多事件/openU |

注意层次：命令的 after 输入是 task graph，没有 caller classifications。旧事件篡改/漏capture的完整快照负例在 inspector/prefix 层测试；不能称为该命令入口直接收到并拒绝了不存在的字段。AST gate配合显式调用链审读，不是任意Go控制流的形式化证明。metadata限制是可验证的逻辑成本边界，不是RSS/耗时担保。

## 验证环境及证据

固定 Go 1.27.3 linux/amd64；通过本副本 `scripts/environment` 显式设置外置 BUILD/GOWORK/TMPDIR/GOTMPDIR，沿用既有cache、`GOPROXY=off`、`GOSUMDB=off`、`GOMAXPROCS=2`、`GOGC=20`、`GOMEMLIMIT=700MiB`、`-p=1`。没有提高 timeout，没有冷cache release或复制toolchain。

作者运行结果由外置 `evidence/runs.tsv` 及各次完整 `.log` 对账；最终摘要见本次交接报告。所有失败运行保留：最初测试先于实现的缺符号编译 red；测试自身的34/36叶计数、顶层节点样本整数截断、多传SubtaskIDs、旧测试工具0-based event index、tombstone须空body，均依真实模型/命令约束修正。未以减弱断言或修改旧r1使之通过。

本批不运行 production/home/iCloud/Keychain/live AI，不做 GitHub 写操作。Swift oracle、非 Linux 实机及新增 helper 的 Service/store 持久化、磁盘故障/双 handle/reopen/Today 发布与 UI 症状验证均未进行。正式整合沿用原有运行路径的项目门禁，不将旧路径通过解释为新 helper 已接线。

## 风险、回滚及下一边界

- 完整图欺骗、错误capture和旧流重写由精确重放+完整inspector+prefix三重围栏阻断。后续接线仍必须重新绑定真正当前snapshot，不能将本结果跨会话复用。
- field/schema变更可能使资源表漂移，由完整模型manifest和optional/canonical成本测试阻断；不能只更新测试清单而忽略生产预算。
- 所有代码未接线，灰度只在隔离合成测试。没有真实流量监控；日志仅合成typed错误、命令、退出码及资源/时间摘要。
- 回滚是整体撤下新增helper及测试/任务文档，旧r1及已有会话不动，不清cache、不删用户资料。
- 独立复核及精确正式树项目门禁已通过。Abandon/Reactivate typed writer、全 persisted frontier、新 development v3 clean cut、store 及 UI 继续独立实施。旧v2不迁移、不覆盖、不删除、不原地续写。

验收用语限于“未接线 typed settlement/complete/undo 精确审计的隔离验证”，不是writer、v3、全跨平台复刻或Swift全量对等完成。


## 正式整合验证（2026-11-06）

独立审查结果为 Accepted，原报告 SHA-256 为 `c869a5890f6b34f4997b71950a9e3b2dd2e6ab6e4009ed41843f09a0658b07e9`。复核方只导入两个新生产文件，未读取作者新测试，独立构造 14 组／94 个含子测试的命名检查；完整 taskapp race（无 `-run` 过滤）164.255 秒，完整 taskapp、聚焦及 vet 均退出 0。独立测试真实覆盖完整规范 envelope 恰好 16,777,216 字节通过、再增一字节由最终 inspector 拒绝的边界。

正式副本导入作者五个与独立复核两个 Go 文件，均与冻结 SHA-256 相同。整合后聚焦测试 `go test -p=1 ./internal/taskapp -run '^Test(HistoryCommand|IC)' -count=1` 退出 0（6.627 秒）。随后 04:10:26–04:25:52 UTC 在同一冻结整合树顺序执行：

- `scripts/check`：退出 0；root UI/race 460.234 秒、taskapp/race 179.106 秒、titlesession 1.399 秒。其他部分包命中既有 Go cache，未冒称全部冷跑；MyGo UI 2.736 秒和窗口退出相关测试 1.059 秒实际通过。
- `scripts/cross-build`：六目标 `linux/windows/darwin × amd64/arm64` 全部编译成功，退出 0。
- `scripts/build`：Linux 构建退出 0。
- `scripts/test-entrypoints`：退出 0；通过开发身份、创建、重开、目录复用拒绝和两页 2400×1536 MyGo 渲染。未测试真实桌面窗口。

门禁前后完整 498 个仓库文件的字节与模式相同；之后只回填本 README 的结果。本批七个 Go 文件与设计/独审冻结内容完全一致。完整命令、退出码、日志、每个产物哈希及冻结清单在显式外置整合证据目录，不将生成产物加入仓库。

本结果只证明未接线 helper 的本地正式整合；不是新 Service/store 写路径、Swift 对账、六平台实机、Hosted CI 或正式发行验收。新的 GitHub 写入仍因旧工具审批结果未明而暂停；没有以重新登录、替代写接口或盲重试绕过。
