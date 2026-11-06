归档说明：以下为冻结设计或独立阶段记录。状态和路径指当时阶段；原文 SHA-256 为 `35502f5e3a706cafb7c068aa86fdc366ccebc3bffe27eafd0417a626affa5493`。整合进度以本目录 README 为准。

# Lifecycle history 独立实现复核

日期：2026-11-06。**Accepted：未接线 typed lifecycle 审计在固定 profile 下通过本轮隔离独立验证。** focused、完整 taskapp、完整未过滤 race 及 vet 均通过。无生产修改要求。

## 独立性及冻结范围

- 正式基线 `3ea3efeaee2a0085576ca1aadd0d3082e643a558`，核实 clean。用该 commit 的 git archive 建立独立副本，只覆盖／加入作者两个生产文件及本审查自行编写的测试。没有读取或复制作者新增／修改测试、fixture 或 capture selectors。
- 读取正式树旧测试用于既有 API、canonical schema 和预算门禁定位；独立正例未调用旧 fixture oracle。`liSeed` 用既有 CreatePoolTask／ScheduleFromPool 形成基础前态；预期变更由 `liAbandon`／`liReactivate` 手写，完整事件由 `liSnapshot` 独立构造，不调用待测 helper／builder 来产生期望。
- `source-verification.json` 对账所有 tracked 文件：唯一改动旧文件是 `history_command_vnext.go`，新增 `history_lifecycle_command_vnext.go` 与 `history_lifecycle_independent_test.go`。r1 inspector、builder、machine、core、profile、Service、store、runtime、v2、UI 和全部旧测试完全保留。
- `auditSettlementHistoryVNext` 起至 EOF byte-exact。最终 command 文件相对初始审查 hash 的唯一额外改动为注释 two→four；将该词替换回去复算得到原始 `0f2bee9e...`。

冻结 SHA-256：
- `history_command_vnext.go`: `a7ba8b24314bffb7503c1278ad0b35b306a015f0c60c23cf9b1907670d336c32`
- `history_lifecycle_command_vnext.go`: `acd9060443ca1c160c72cc8346d8074ac2737de7c5e9b537fc7757caef41b4dd`
- 独立测试：`12fc2f5c125de7fb8f163fba3c1bf70bfae91d53bfa0d9b5300fdd9902269ab1`
- `independent-tests.patch`: `b055cb3cb262bbd5b5376177ca9685f5d2e90be1ea77da978d60e81c4180083b`

## 源码裁定

未发现功能阻断。

1. Abandon 在实际 pending 重定向之前核对请求 trace 的 owner；不存在 unfinished-as-active 选择。effectiveTrace 从 owned prior 推导，core replay 后再次核对，恰一 A intent 绑定实际 trace／Now。
2. Reactivate 只操作显式请求 A；没有 latest-A 规则。锁日或 past 用 U 且保留 A settledAt；open current/future 用 P 且清 settledAt。capture count 从 owned prior 分支确定，P 零、U 一；调用者不能提供 status／trace／clock。
3. 两入口前四个直线 if 为 prior seal、before budget、after budget、ID metadata。Reactivate count 在任何 EventIDs 索引前；只读取有界字符串长度。复用原模型预算，不引入另一套成本模型。
4. 绑定使用 original supplement、完整 before canonical 比对与 prior round-trip；重放从 owned prior 开始，只调用对应 core typed command 一次。after 按整个 graph 比对，不按目标字段子集比对。
5. 复用的 finish 顺序为 append→compose→完整 inspector→prefix→supplement/count fence。原 supplement 与所有旧事件不会洗掉；额外生命周期 fence 固定冻结 days/definitions/subtasks 与非目标 owner/trace；P 进一步冻结全部 classifications。
6. closed proof 仅增加标量 request/effective/event 身份及两个穷举 kind。没有可变 EventIDs slice 或 caller graph 引用泄露。所有失败分支返回零结果；返回 copy 是 owned copy。

源码引用：新 lifecycle 文件全部；旧 `history_command_vnext.go` bind/match/finish 与预算文件；core `history_chain.go`；Swift `NoonmarkEngine.swift:2513–2571`、`TaskClassification.swift:4022–4056`。已读 AGENTS、port DESIGN、上游领域词汇及 ADR-0004/0020/0022/0027/0037，遵守隔离授权对历史主机路径的替代，不改变生产／iCloud 安全边界。

## 独立运行覆盖

最终 focused 有 8 个顶层测试，连同子测试共 52 个 PASS 行。

- P→A 完整 graph/events、proof scalar、lowercase 新 event ID 身份与 uppercase 新事件表示。
- current/future open A→P，past open A→U，locked past/current/future A→U；U 的 capturedAt/contentUpdatedAt 与保留 settledAt 分离；P 零 capture 仍 changed。
- 同链两个 A 从同一 prior 分别选择两者成功；同链两个独立根 U 分别废弃请求者成功。这些明确是结构 fixed-profile 参数，不声称 Service 可达。
- 真实 continuation 的 historical U 请求重定向唯一 pending，requested/effective 身份均保留；真实 U predecessor 在存在 descendant 的情况下改 A 被拒绝，未放宽 continuation validator。
- 反复 A→P 循环；A→past U 后首次 settlement 不重复 capture；future A→P→Return X 的完整 inspector 明确拒绝保留 A 的 X。
- 错 owner／missing／invalid IDs／nonfinite／局部时钟回拨／before binding／after delta／inactive chain／non-A；P 多 ID、U 缺 ID、重复与大小写重复 ID、36-byte 非 UUID、count>1、非法 Today、错误替换 U settledAt。错误结果均不携带 proof，changed/snapshotCopy 均拒绝；输入保留。
- 完整 rich graph：pending/completed/cancelled child、planned child、说明、手动进度、置顶、普通附言／墓碑附言、无关 chain。十个仍通过 core Validate 的恶意 after 改动均拒绝，包括 planned child、actual child、note、progress、day review、无关 clock 和 array 顺序。
- 两图 budget 优先于非法 domain 内容与 IDs；ID gate 优先于 bind；AST 门禁限定前四步、立即零返回、bind 位置及 ID metadata 调用集合。原 exact-model／nested／optional／shared-leaf 成本测试由未改动完整旧 suite 继续覆盖，不将新 AST 门禁称为形式化资源证明。
- negative fraction、subnormal、+0、-0、MaxFloat，exact bits 成功；数值相同但 signed-zero after bits 被改也拒绝。无关 chain 较新 clock 不被误当全图 mutation frontier。
- 旧 classification lowercase key/event 字节保留，新 event 规范化；两个各自合法端点改写旧 prefix 被拒绝；task rawValue round-trip 变化拒绝绑定。
- returned-copy 变更、caller graph／ID slice 后续变更不影响 proof；共享 immutable prior 并发独立审核／copy。不会据此宣称调用中并发修改 caller 输入安全。

## 执行记录及限制

串行、离线、Go1.27.3、warm shared GOCACHE；GOMAXPROCS=2，GOGC=20，GOMEMLIMIT=700MiB，-p=1。每阶段 metadata 保存具体命令、三个输入 hash、Go 版本、时间和可用容量。`run-stage.sh` 是可重复执行入口，先 source 原 scripts/environment；没有 cold release／六目标构建，没有缓存删除。

第一次 timing wrapper 调用 `/usr/bin/time` 时发现该工具不存在，exit127；Go test 没有启动。保留 `focused.log`/`focused.exit`，随后改为 Python subprocess/monotonic 计时。此是执行包装修正，不是测试 red 或生产修改。

通过的第一版 focused 后追加时钟及表示测试，再运行最终 focused。第一版记录保留，不用旧 hash 的通过代替最终运行。

最终四项全部 exit0，详见下列记录。当前验收范围仅未接线 typed lifecycle helper 的固定 profile；不证明完整历史可达性、当前 head、写入授权、全局 mutation frontier、Swift oracle、非 Linux 实机、Service/store cutover 或 v3 writer。


## 最终实际执行结果

全部命令均在独立副本 ports/mygo 执行，使用最终冻结测试及生产字节；完整 taskapp 与 race 均没有 -run 过滤。

- `go test -p=1 -count=1 ./internal/taskapp -run '^TestLifecycleIndependent' -v`：PASS，package 0.188s，wall 4.295s，exit0。日志 `focused-final.log`。
- `go test -p=1 -count=1 ./internal/taskapp`：PASS，package 29.774s，wall 30.120s，exit0。日志 `full-taskapp.log`。
- `go test -race -p=1 -count=1 ./internal/taskapp`：PASS，package 178.377s，wall 186.268s，exit0，max RSS 387704 KiB。日志 `full-taskapp-race.log`。
- `go vet -p=1 ./internal/taskapp`：exit0，wall 0.740s，无输出。日志 `vet.log`。

每项的 `.metadata`／`.time`／`.exit` 构成对应执行收据。`independent-tests.patch` 是只新增本轮独立测试的冻结补丁，不包含作者测试，不修改生产文件。正式树未写入；本报告不授予合并／发布授权。
