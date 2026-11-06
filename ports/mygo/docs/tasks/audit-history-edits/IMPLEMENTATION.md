> 阶段归档：保留原设计或隔离副本验证边界，正式整合状态见 README。

# 标题与手动进度纯历史审计实现

2026-11-06。基线 `2a808a3c4fe788c69d8f47cf21e1d686860a19ff` 的本地精确 archive 候选。没有改正式源码、远端、production、iCloud、Keychain 或 AI；没有删除缓存。此批没有接 Service/store/UI/v2，没有 writer、allocator、迁移或 clock frontier 权限。设计独审 Accepted 不等于本实现已获独审。

## 实现范围

- 新 `history_edit_command_vnext.go`：title 的 Rename/Input 和 chain/trace 闭合路由；manual progress 原核 replay；全图 match、finish(nil)、固定字段 fences；私有标量请求身份。
- 新 `history_edit_command_vnext_budget.go`：两个固定大小 metadata 输入预算；独立 phase=4。所有输入 string 每逻辑字段计费，未使用 NewDefinitionID 只计字节，不检查内容、UTF-8、UUID 长度或冲突。
- 旧 `history_command_vnext.go` 只有三 kind、五项 private scalar metadata、四→六入口注释及 valid 白名单注册。四个旧 audit、bind/match/finish/budget/core/runtime/store/r1/v2 保持原样。
- 旧测试只扩展新入口 AST 顺序和未知 kind 边界，不删旧断言。

## 关键解释

1. 两个入口均是 seal → before budget → after budget → input budget → immediate bind。输入和两图各自独立预算，不混成一个16MiB，也不按最坏6倍转义估计拒绝。
2. manual 36字节仅 metadata 形状。内容存在性、owner、Today/锁日/状态/child、floor与时钟顺序由原 SetManualProgress 决定。36字节坏 TraceID 返回原 ErrNotFound，wrong owner 返回原 ErrInvalidInput；没有新增替代 typed owner。
3. title trace route 的 owner 是 typed owner，缺 trace/历史日期/非P分别拒绝。chain route 不强求 trace，保留池和R。
4. No-op 不调用 Now.Finite，也不计算 chain max。私有 now/unused ID 是请求身份，不是消费或分配凭证。真实变更 chain clock 用严格 Now.After，±0数值tie保留旧bits。
5. 固定 fence 使用从 sealed prior decode 得到的 owned graph 私有 Clone，并只投影固定字段，再整图 reflect.DeepEqual。它不接 caller allowlist，也不拿未规范化 caller before 做反射比较。planned nil/empty 在 wire 上都是[]，owned decode统一；optional description缺席与空字符串仍区分。额外固定 snapshot key 集、capture count、非task supplement；finish保留完整 inspector/prefix。
6. Manual/Title 任何错误返回零 proof。成功结果只有 owned sealed endpoints与私有标量，没有共享可变 request。snapshotCopy 深复制。

## 验证与失败记录

外部证据根：`/workspace/shared/noonmark-history-edit-vnext-20261106`。每次日志包含精确命令、UTC start/end、exit。`red.log` 在任何新生产实现之前真实编译失败，缺少新类型/入口。不是伪造断言失败。

- focused-1/2：代表入口、clock/no-op、预算、旧AST扩展通过。
- focused-3：两个新fixture失败，restored witness指向不存在定义、R缺capture。改为真实同链既有定义witness与明确core-only R capture；未改生产。
- focused-4：修正后通过，包含真实 typed 命令谱系。
- focused-5：planned fixture错误Position0，被core invalidPlannedSubtasks拒绝。源码validation.go明确1..len，依既有titleRichGraph改为1；未试探或修改领域规则。
- focused-6：通过。第一次taskapp-full通过31.038s，但不覆盖随后新增测试。
- focused-7：新增locked P被full inspector拒绝unsupported profile chronology。不能虚构sealed prior。改为独立断言profile拒绝，再单测原core锁日no-op guard；未扩大profile。
- focused-8：最终新增测试通过。其后源码冻结并重新执行final-full、未过滤race及vet；最终结果见外部日志和RESULTS.md。

## 测试证据分层

命令合成谱系 `TestHistoryEditCommandGeneratedLineages` 的[A]P/[A,A]P/[A,U]/[U,A,U]逐边调用真实core与已有typed audit，每次full inspector/prefix后才前进。历史U无P/R标题拒绝；异链真实当天P继续manual/title；全事件字节/count不变。没有将vnFixture的手工capture叫作typed可达路径。

R/return/schedule、subtask以及continuation使用真实core命令，但这些边尚无本批typed adapter，标作core-only fixture。restored witness、clock bits、R引用current及planned字段是sealed-valid参数/profile fixture，不声称Service可达。locked P不能成为此profile合法prior，测试分别证明profile拒绝与core防御guard。

新增测试覆盖：pool/P rename与reuse；输入归一化、长combining、canonical no-op保存原bytes；unused非法ID/UTF-8/非finite clock；equal/negative/±0/newer-chain clocks；route/owner/状态错误；version冲突/sequence overflow/restored witness；manual clamp极值/nil→0/no-op/历史floor/同组peer/continuation/complete→undo；全链过去/同日/未来child、cancelled和异链child；optional description/planned复制；before绑定、整图malicious after与独立fence；快照别名、scalar shape/unknown kinds；预算precedence/边界/alias累计；AST和冻结门禁。

TEST-MATRIX.md保持原“待实施测试矩阵”的设计证据，不把每一行自动标成完整覆盖。近envelope节点/overflow算术与prefix篡改由不变旧budget/builder/inspector套件覆盖；本批没有独立重新实现这些算法。新增套件没有专门构造同日较小continuationSeq但不同group的profile，也没有额外新增所有事件键大小写排列或所有字段mutation笛卡尔积；后续独审可要求补例。既有未过滤全包与race仍包含旧相关断言。

## 未完成边界

尚未独立实现审查、Swift Foundation oracle、Apple/Windows/arm64实机、真实UI IME/owner切换/保存退出、manual交互或未来writer接线。本轮Linux纯证明不构成这些结论。不得把proof.now或snapshotCopy当mutation时间或提交权限。
