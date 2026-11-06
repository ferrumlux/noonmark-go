# 未接线计划／实际子任务历史审计

Task-Id：audit-history-subtasks。风险 P。日期：2026-11-06。

状态：作者新 focused、完整 taskapp、完整 taskapp race 与 vet 通过；只读实施审查接受纯 helper 实现。仅载入三份 production 的独立行为验收也已通过；已整合双方测试并完成全量聚合，见末节。本批不接 Service/store/UI/v2，不新增 writer、allocator、迁移或已启用产品功能。

## 需求、基线与设计

在既有 sealed history／完整重放框架内，补齐 planned 四动作（add/title/difficulty/removePlanned）与 actual 七动作（add/title/difficulty/delete/complete/undoComplete/abandon）的封闭纯审计。既有子任务能力、错误次序、历史定义、取消见证、精确 clocks 与零 capture 完整历史检查均不能损失。

实施基线为 `c2ca275d8e43975b82f71a633decfe6d0dc88b97`，tree `d9bb41e89346d726a4b7ddb6a885a28f9638a60d`。在独立本地分支实施，没有修改正式树或远端。父级其后将共同基线更新至 `e35a5d5d628b7c0d0a2ece945fc18e944d344ee0`；该批为 Swift oracle／CI 合同等独立变更，本实施没有盲 rebase，整合与最终共同门禁由父级另记。

恢复后新设计的独审裁决为 Accepted，仅接受纯 helper 设计。设计 SHA256 为 `d0347b77ea48187dd7e4810238db756f7e81882dd26d1a02f630e4145cb10647`，矩阵为 `139f397ffaea485c585f752789dfcdd25e2ddf7b26e246753bf556a5c1086e57`。这不是旧文档恢复声明，也不是执行证据。本文保存必要语义、仓库来源与实际结果，不依赖外置绝对路径恢复任务。

主要可重读来源（相对 `ports/mygo`）：
- `domain-core/core/subtask_edit.go`：原 guard、shape、no-op 与变更顺序；`models.go`、`validation.go`、`return.go`、`carryover.go`：身份、历史范围、位置和 continuation 约束。
- `internal/taskapp/history_command_vnext.go`：owned bind、完整 match、finish 与封闭结果。
- `internal/taskapp/history_vnext.go`、`history_vnext_machine.go`、`history_vnext_builder.go`：full inspector、NFA、prefix 和零 capture builder。
- `internal/taskapp/subtask.go`、`subtask_alias_review_test.go`：旧 writer 及原 historical-definition alias 边界。现有 writer 完全未改。
- `domain-codec/v7/domain.go`、`schema.go`：canonical bridge／optional presence。`draftCancellationID` 的 wire 类型为 uuid 字符串，不是 `ID.rawValue` 对象。

## 接口与预算顺序

仅新增 `history_subtask_command_vnext.go`、`history_subtask_command_vnext_budget.go`。两个按值 input 分别与 `core.PlannedSubtaskEditInput`、`core.SubtaskEditInput` 同构；新身份均由 caller 显式给定，不分配 ID。两 family 使闭集从8 kind／6入口变成10 kind／8入口，没有11种顶层kind或nullable union。

共享 proof 只增加 action、expected definition、planned/actual child、lineage、difficulty、cancellation 七个私有 scalar，复用原 requestedTitle 和 phase4。请求 Now 保存原 bits，不代表 no-op 消耗过时钟；proof 只证明这一条完整边，不代表磁盘 head、provenance、session 或提交授权。

每入口固定顺序：
1. prior seal 有效。
2. before graph 结构／raw metadata 预算。
3. after graph 结构／raw metadata 预算。
4. 全部 input strings 的累计 raw 长度预算。
5. 立即 bind，然后才允许复制、验证、内容扫描或 core replay。

before、after、input 分别独立受限，不将三者相加误拒。新 input 仅调用既有 raw 计费：planned 的 Kind/ChainID/ExpectedDefinitionID/SubtaskID/LineageID/Title 六字段；actual 的 Kind/ChainID/TraceID/SubtaskID/LineageID/Title/CancellationID 七字段。Kind、unused 字段及共享 backing 的每次逻辑使用都计入总量。没有提前 UUID、36字符长度、UTF8、normalize 或 Now 验证。超总量返回 phase4／RawBytes；合限非法字段仍由 core 原 guard 次序拒绝。

## 重放、错误次序与固定护栏

预算通过后，从 owned sealed prior 解码；一次调用唯一对应 core 命令，不从 caller before 开始变更。core 原失败直接返回零结果，包括 required ID/shape/freshness、标题、owner/date/lock/status、child 和最后 clock 的先后关系。

完整 match 比较全部 canonical graph。invalid caller after 可先由 bridge／validation 返回原错误，只有两个成功编码但不相等的完整图才是 after mismatch。随后无条件 `finish(nil)`：append builder、分类深复制、compose、full inspector、prefix、supplement 与 exact capture-count 全部执行；no-op／零capture没有捷径。

两个独立 fence 从 prior 构造固定 canonical 期望，再全图精确相等比较。未知字段、数组顺序、所有未列字段、optional absent/empty、分类pair与事件、Instant bits均冻结，不接受 caller 自选豁免。
- planned：仅目标child字段及definition.ContentUpdatedAt／chain.UpdatedAt可按动作变化；两个clock写exact Now。add只尾加；remove物理删除并按旧数组相对次序重编号全部Position。不是取消墓碑。
- actual：父chain、trace、definition全冻；仅目标child规定字段及UpdatedAt可变。add的Position包含隐藏X。delete保留身份／lineage／位置／创建时钟／carried identity／内容，只写X、清CompletedAt、写SettledAt／UpdatedAt与fresh取消见证。其余动作保持未列终态字段及合法恢复见证。
- title canonical equivalent／difficulty相等是真no-op，全snapshot不变，可携带NaN/Inf、旧Now等未消费请求值；前置shape/owner/date/lock仍须通过。真实变化允许局部equal、负数及±0，不增加全局floor或strict successor。

### historical-definition alias

planned另冻before中所有 `FormsDayHistory()` trace引用的definition完整canonical字节，范围包含P，只有X排除；ContentUpdatedAt也不能豁免。不能借父标题投影允许历史定义改写。

原 `reviewUndoContinuationAlias` 先在seal的historyToday边界拒绝，不能假称命中fence。新U→X提前取消日及R/current两种参数fixture已逐层证明：before full inspector、core replay、final full inspector、match和finish全部成功，再由额外历史definition fence拒绝四种planned变更。它们是合法sealed参数结构，**不是实际命令可达或用户bug声明**。同alias真title/difficulty no-op允许；真实Return生成fresh current definition后编辑允许，不能冻结同chain所有定义。

## 变更保全

预编辑保存19个taskapp production文件及133个旧函数body。核验132个body原样，唯一例外为valid显式增加两个kind case。旧 `auditSettlementHistoryVNext` 到EOF保持原SHA256 `ac1d9c4a57753c20a3a906bbad64e961cd92bd115304528ccb7e2e8a8854b17d`。

另从不可变c2 Git对象后验核对全`ports/mygo`：76旧production Go文件中只command注册文件改变；749旧production函数中只valid改变。包含tests时为238文件、2078个旧函数，只有valid与5个测试函数改变。该全仓核对不是预编辑全仓freeze声明。仓库原冻结manifest未重生成。

旧测试只扩展新入口／文件或10kind闭集：budget AST保留原六入口所有断言；四个closed-kind测试加两合法kind，并将“最后合法+1”哨兵移至ActualSubtaskEdit+1。0及11..255仍拒绝；全uint8枚举、旧断言保留，没有skip或删除测试。

冻结production SHA256：
- command注册：`0e528776f7bef031e9c680252a650034bbeb0ee84a5e591b20f286f5ced2fef9`
- subtask command：`2093c2b7793fabf2f2ecb06243d4a06328ba50f36d9ebe369c89877c75a83c23`
- subtask budget：`ee585652cdcc2044ab65b015faec7948f077d53885a2521dcfcc2b8a02db311e`

## 实际验证与可重复入口

使用官方Go1.27.3、独立外置BUILD及已核离线依赖，只运行合成资料。相对`ports/mygo`的实际Go命令和最终结果：

| 命令 | 结果 |
| --- | --- |
| `go test -count=1 ./internal/taskapp -run '^TestHistorySubtask'` | exit 0，2.965s |
| `go test -count=1 ./internal/taskapp` | exit 0，34.993s |
| `go test -race -count=1 ./internal/taskapp` | exit 0，218.604s，无race报告 |
| `go vet ./internal/taskapp` | exit 0 |
| `go test -count=1 ./internal/taskapp -run '^TestHistorySubtaskActualIndependent(Fence|Oracle)$'` | exit 0，0.095s |
| `go test -count=1 . -run '^TestFailureGateRegistry$'` | exit 0，0.091s |
| `git diff --check`；独立index对c2的完整patch apply check | exit 0 |

后续复跑须先显式选择官方Go1.27.3的 `NOONMARK_GO` 与现存源码外 `NOONMARK_MYGO_BUILD_ROOT`，准备经仓库manifest校验的固定依赖，然后使用 `scripts/environment` 的目录／依赖守卫运行上表命令。不要用系统旧Go、源码内缓存或删减原门禁求绿。本文新增后仅文档改变，生产／测试仍与上述执行版本一致。

实际新覆盖包括：
- 11动作独立常量与字段oracle；planned首中尾／空数组／合法乱序Position；actual交错global数组、隐藏X计数与恢复witness。
- 每kind shape、消费／unused标题、UUID／枚举、owner／日期／锁／parent与child状态、全图freshness及预算错误优先级。
- no-op原始NaN/Inf/MaxFloat；11动作equal／负数／±0／MaxFloat exact bits；planned两clock与actual child-only clock。
- 每输入字段MaxBytes/+1及扣除其它字段长度后的真实总量边界，shared string重复计费、三独立预算、AST四gate／立即bind／core一次／finish(nil)／仅成功构造。
- 7种历史A-P/A-A-P/A-U/U-A-U/R/D/C，各覆盖异链planned4、当前actual7、未来actual4；真实ContinueUnfinished显式child映射后actual7保留carried身份和旧历史。Continue/Return/Defer fixture为core加独立capture/inspector，未称全typed。
- 成功／失败后caller共享planned slice、notes、optional pointer污染与返回copy深层污染，不影响owned prior/result。审计过程中并发修改caller输入不在支持范围。
- 两专属fence逐动作合法delta和未知字段、classification revision/latest/pair/bits、父clock、数组顺序、siblings／lineage／witness／carried identity污染。

候选delete护栏真实缺陷已独立登记[GO-HISTORY-SUBTASK-001](../../engineering/failure-cases/GO-HISTORY-SUBTASK-001.md)，状态保持处理中／修复commit待整合；fast/symptom对应两真实taskapp测试，沿现有scripts/check接入，未覆盖或改写父级新增的Swift oracle登记。

保留的红→绿证据不是省略失败：首测试在API未实现时编译红；独立fence/oracle抓到首稿delete cancellation wire对象错误，按schema字符串根本修复；新fixture的nonfinite构造、取消ID碰撞、Continue合法chain clock及note墓碑先后被原core约束抓到，修正fixture而非放宽核心；首完整taskapp的旧kind哨兵经批准扩展闭集后绿。首race使用中间非法note fixture而红，最终完整race重跑绿，不能把前次红称为race通过。

只读实施审查在独立BUILD执行实施团队tests，最终focused exit0，3.098s，并核三production与旧函数冻结。该审查**不是**未见测试的盲写行为oracle；父级独立验收及整合门禁结果另补，不预写通过。

## 风险、回退与未完成

主要风险为预算漏计、guard次序变化、parent/child clock混淆、alias历史改写以及remove/delete混淆；以上由新测试与旧完整taskapp回归守门。回退只撤两新helper和封闭注册，现有writer及v2数据没有切换或迁移，不需要回滚用户资料。

没有穷举所有状态×日期×锁日笛卡尔积；不合法prior组合不能伪造seal测试。MaxInt实体容量未实际分配，继续依赖checked预算/溢出门禁。没有新Apple Swift Unicode运行oracle、多OS实机、真实UI、耐久性、四fault点、writer/frontier、磁盘no-op prefix或11消费者接线验收。本worker未跑全项目UI/check或release；父级需在共同正式基线整合后运行原全量fast gates。未读production/home/iCloud/Keychain，未作真实AI调用。

## 共同基线整合检查点（2026-11-06 UTC）

在 e35a5d5 共同基线的独立整合树选择合入上述冻结源码、测试及故障登记，完整保留既有 Swift gate。设计43项来源哈希现场重新核对一致；c2→e35仅独立oracle/CI变更，没有改子任务设计依据。六目标交叉构建先行退出0，构建期间545个源文件及mode不变；该结果只是编译，不是目标系统运行。

另在仅带三份production的新e35 clone独立写11组对抗测试，没有读取或复用作者新测试；最终focused和race均退出0（race2.764秒）。测试已收入本树，具体覆盖与未覆盖见INDEPENDENT-ACCEPTANCE.md。完整taskapp/race的作者结果不替代该独立验收，也不替代接下来的全项目聚合。549份源码及mode全程冻结，scripts/check退出0，根/UI race541.422秒、taskapp race244.296秒，另含四模块vet、Unicode来源/路径/编译、51项dependency及原生输入/popup门禁。没有改源求绿。之后只选择合入已独立验证的Swift证据文档、两表状态及一条精确Python断言，不改任何生产Go；最终受影响root gates已以-race -count=1重跑退出0（32.832秒，包含28项比较器回归、19项oracle合同及registry/BUILD守卫）；六目标、native build与隔离CLI均再次退出0。CLI实际render day/pool 2400×1536、create/reopen/reuse与无隐式会话拒绝通过，仅headless，不是桌面/IME验收。本批仍不接任何runtime writer。
