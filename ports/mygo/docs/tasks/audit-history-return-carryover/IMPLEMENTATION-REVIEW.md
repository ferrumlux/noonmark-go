# Return / Defer / Continue 修订实现独立审查

日期：2026-11-06。结论：**Accepted，仅限未接线 pure helper 实现范围。**

未发现新的阻断实质缺陷。接受对象为候选本地 HEAD `32befa59455f74078aeded954ba96578b2092fdb` 相对本地 snapshot `4f11424` 的四 production 文件与明确注册修订，不是远端认证、正式整合、完整状态笛卡尔覆盖或产品上线批准。原 `2d76afb` 的 kind/detail 错配缺陷真实存在；原 race 绿不能作为其安全通过证据。本接受仅适用于修订后字节。

## 方法与来源

逐段阅读冻结设计、原独审及 `PROOF-CLOSURE-CLARIFICATION.md`；检查三新 production 文件和注册完整源码，独立对照原 `domain-core/core/{return,carryover,kernel,time}.go`、现有 bind/match/finish、builder、inspector 和固定 Swift 源文件。Swift 读取位置为 `subtask-history-design/upstream-43e6eef/Sources/NoonmarkCore/NoonmarkEngine.swift` 的 2019–2065、2152–2270、3805–3812、4087–4101、4124–4153、4180–4240。这是已提供固定字节的语义核对，不是重新认证上游远端或运行 Swift oracle。

独立重算四 SHA，全部匹配最新 production-freeze.json；注册 SHA 为 `90e22cf119936f446913e9441b0e4cbbedd02ba97d9da9d79917b72798a92363`。完整结果、HEAD、clean status 和差异清单见 `SOURCE-VERIFICATION.json`。旧 base549 与正式 ed550 的 Swift 证据文档差异不被抹平，本报告不声称二者同一 tree；只评价指定实现 delta。

## 实质核对

1. **预算先行与顺序。** 三入口均直接依次 seal、before graph budget、after graph budget、input budget、bind。map 先以 sealed subtasks 长度与 MaxNodes/2 限 count，再 checked 2*N，之后才遍历 key/value headers；所有标量字符串含 unused CancellationID/DayID/EventID 均计 raw bytes。copy、kernel、bridge、UUID 内容判断没有前置。预算内未增加 owner/Now 等抢错 guard，原 core 错误按原顺序透传；after mismatch 早于 capture cardinality，builder 与 inspector 错误未折叠。
2. **完整 replay 与 X 拒绝。** 成功的 owned replay 决定 R/X，R 和 D 仅 source 各一 capture；X 要求空 EventID，Continue 无 EventID 输入。finish 始终 append/compose/full inspector/successor，不将中间 classification 候选当 endpoint。future A→P 携旧流的 X 不清旧流，而在完整 inspector 拒绝。core mutate 仍保留发布前 ValidateTaskGraph。
3. **精确历史追加。** 独立 capture fence 从 prior classifications clone 建预期：旧 stream 完整 prefix 追加、旧 latest 原位替换、新 key 才尾加 pair，两字典自己的 pair 次序保留；revision 精确 +1，event 的无 category、空 labels、Now bits、source/status 精确。EventID 的 UUID、全局 freshness、容量继续由 builder 负责。X/Continue classifications 与 count 整体冻结，不搬 source 历史。
4. **独立 fence 不是第二次 core replay。** 它从 prior 解码 graph 和 typed request 构建固定预期，再完整 canonical 相等比较，没有调用 kernel mutation 或从 after 反推预期。它确实重复部分字段推导，但这正是固定字段边界检查，而非再拿 replay 比 after；唯一共享领域计算是设计明确要求的 NewProgressProjection。完整 graph 重编码冻结未列字段、旧位置和 optional，且由 bind 的 exact roundtrip 排除 prior normalization 漏洞。fence 不重复所有 eligibility guards，资格仍由唯一 core replay 判定，不应宣称 fence 单独可认证命令。
5. **Return 字段。** 新 definition 由旧 current title、source description/notes、BEFORE P/U children 投影，旧 current 仅 supersession/content clocks 变；source DefinitionID 不转挂。future 先取 open children，再还原原 X child，后者不混入 planned。R children 全冻，X 仅规定取消字段、状态与 clock 变。
6. **D / Continue 字段。** 新 trace 使用 current definition、source projection、active notes；新 children 保 lineage/carryfrom 且排除 terminal；D 仅源 P/open children 的明确字段变，Continue 所有旧 trace/child 全冻。旧 day 不动、缺目标 day 才追加，definitions 全冻，chain 仅条件 UpdatedAt。
7. **数值和位。** 目标 priority 有 hasPriority，不把负 priority 错误 floor 到零；Now 无 +0 floor、无 global clock guard、无 nextUp。D/Continue 用数值 After 更新 chain，相等保旧 bits；新实体、capture 和赋值字段保请求 bits。Return 的局部 source/chain/definition/hidden-child clocks 留给原 core。未把 Go 显式 IDs/owner/finite/overflow 的错误次序冒称为 Swift 同构接口。
8. **拥有权与闭集。** budget/bind 后复制 map；私有 detail 保留全部请求 scalar 和 owned map；snapshotCopy 深拷贝。调用期间 caller 并发变异不在合同内。旧十 kinds 必须三 detail 全 nil，新三 kinds 只能有匹配单 detail；invalid 同时被 snapshotCopy/changed 拦住。旧成功入口未新增 detail 赋值，原 seal/endpoints 条件仍在。修订只在允许的 valid 原函数收紧闭集。
9. **白名单。** 相对本地 snapshot 仅修改一个旧 production 文件的 kind 常量、proof 字段、注释及 valid；新增三个 production 文件。原 Service、core、codec、store、旧 audit 函数与 graph budget 未改。另有新增 tests/docs/failure gate，原 budget AST test 仅扩展入口列表与对应 helper 期待，未降低旧断言。

## 本次运行证据

在本审查目录新建独立 external BUILD，source 现有 activate.sh，使用官方本地 go1.27.3，独立 GOCACHE、指定共享 modulecache、网络依赖下载关闭。未修改候选源码。

命令：`go test ./internal/taskapp -run 'Test(Remaining|HistoryRemaining)' -count=1 -v`。

结果 exit 0，27 个顶层测试通过，日志 `existing-focused.log`。覆盖现有作者的三入口/四路径、预算/guard、direct fence mutation、时钟 bits、闭集矩阵等。已读真实成功矩阵代码，十个旧 kind 经实际入口构造，R/D/Continue 也经实际入口构造，再做新旧错配与 detail presence 矩阵。

**这是审查者独立运行现有测试，不是本次新盲写独测。** 另读提供的 `remaining-history-independent-revision-build/REPORT.md` 作为既有证据背景，未把其 focused/race 当成本次运行；本次没有跑 full check、全包回归、race 或 vet，也没有重写/覆盖原红灯。

## 限制与后续

不声称完整 guard × status × child × history × capacity 笛卡尔覆盖。未验证 Service 接线、writer/head/session/全局 clock、v3 clean cut、持久化故障与重开/双 handle、Swift 实机、macOS/Windows/UI 或真实用户资料。所有运行仅 pure helper 测试，未触 production/home/iCloud/Keychain/live AI，未写 GitHub/CI。

正式整合时应在同一最终 HEAD 重新执行所需完整验收，不能用两个分支分别绿替代整合绿。接受本批纯 helper 不授权接线或放宽现有 fail-closed scope。
