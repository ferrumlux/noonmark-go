# 普通任务链核心：隔离验证

task-id: connect-history-chain-core

## 交付与范围

从 `7c239df6941f8e6f923b078da6e6ba5ce8dc10b3` 的 Git archive 建立 sibling 独立副本，只新增 `core/history_chain.go`、`core/history_chain_test.go`、本目录 DESIGN.md 与 VERIFY.md。没有修改现有源码、validator、Service、codec、投影、UI 或 empty-catalog reader。无 commit、push、远端写入或 release。

需求与先于实施的 A 级设计、Swift 精确来源、接口、风险、分类捕获保留边界及回滚方案见 DESIGN.md。Abandon 成功只表示其实际目标已改变；历史请求可能保留，未来 pending 才是目标。Reactivate 成功返回同一指定 ID，失败返回空 ID。

## 源码冻结 SHA-256

- `ports/mygo/domain-core/core/history_chain.go`：`9ef3cc08e08ff42bdb39679b792b7f2a929e049905ee95e834a4c2f022b546b8`
- `ports/mygo/domain-core/core/history_chain_test.go`：`919dfbc3db2dfab0898779ef0ed4d5a9fc2a66b600c8b991232f50c163df027f`
- `ports/mygo/docs/tasks/connect-history-chain-core/DESIGN.md`：`fad30c3cc7226b9d70d86c4970701869136327dbb892a1b1205f21da4db8c82d`

VERIFY 本身、上述文件以及 patch 的最终哈希保存在外置 BUILD 的清单，不在本文件内循环记录自身哈希。实现和测试在首轮 focused 后没有更改；设计在验证后仅补充分类测试来源、明确请求/目标语义及 Swift 拓扑来源。

## 实际执行

工具链为固定路径的 `go version go1.27.3 linux/amd64`。所有命令使用 `GOPATH=/tmp/noonmark-gopath`、共享固定 GOCACHE、`GOMAXPROCS=2`、`GOGC=20`、`GOMEMLIMIT=700MiB`、`GOPROXY=off`、`GOSUMDB=off`、`-p=1`。GOTMPDIR、coverage 与日志均在 `/workspace/shared/noonmark-history-chain-core-20261106`，没有复制工具链、缓存或清理既有内容。

全部下列运行退出 0：

1. domain-core：`go test -p=1 -count=1 -run '^TestHistoryChain' -v ./core`。13 个新增顶层测试，含日期/锁日、八状态、链状态、精确时钟与错误路径矩阵；0.011 秒。日志 `logs/focused-1.log`。没有先红后改测试或丢弃失败日志。
2. domain-core：`go test -p=1 -race -count=1 -coverprofile=<BUILD>/domain-coverage.out ./...`。core 20.956 秒、93.8% statements；textcanon 1.690 秒、100%。日志 `logs/domain-race-1.log`。
3. domain-core：`go vet -p=1 ./...`。退出 0，空日志 `logs/domain-vet-1.log`。
4. domain-codec：`go test -p=1 -count=1 ./v7 ./canonical`。v7 0.230 秒、canonical 0.005 秒。日志 `logs/codec-compatibility-1.log`。
5. 根模块：`go test -p=1 -count=1 ./internal/taskview`。0.012 秒。日志 `logs/projection-compatibility-1.log`。
6. 根模块：`go test -p=1 -count=1 -run '^TestEmptyHistory' -v ./internal/taskapp`。0.102 秒，包含 unsupported abandoned event、历史完整性与非空 catalog 拒绝。日志 `logs/empty-history-boundary-1.log`。

上述第 4–6 项是现有兼容/边界回归，不是新命令已接通 Service 或跨引擎 roundtrip 的证明。完整 root 聚合、release 与跨平台编译不在此独立批次重复执行，由整合阶段组合后另验。

## 行为证据

- 正常路径由 CreatePoolTask、ScheduleFromPool、EditSubtask、SettleTaskDays、ContinueUnfinishedTrace 和 MarkCompleted 产生。额外结构 fixture 明确标识并先通过构造器/ValidateTaskGraph，未绕过图不变量制造公开路径。
- 精确 whole-graph 预期证明只修改 chain.state/updatedAt 与实际目标 trace.status/contentUpdatedAt/pinOrder/settledAt。其余 day、definition、历史源、subtask、所有 note（含墓碑）、description、manual progress、cancellation witness、身份和实体数量保持。
- pending/unfinished 废弃；历史请求重定向未来 pending；请求与目标不同；错链先拒绝；所有终态直接请求拒绝但同链 pending 重定向可成功；重复废弃、reactivate 状态与活动目标冲突；其他 chain 的 pending 不误阻断。
- reactivate 的过去/今天/未来 × 锁日矩阵，锁日优先于自然日倒退；原 ID 不复制；unfinished 保留旧 settledAt，pending 清除它；pin 不复活。
- 有限相等、负数、小数、正负零、最小非零值与正负 MaxFloat64 位模式；输入 NaN/Infinity、trace 或 chain 单独倒退拒绝；其他未修改事实或请求历史源的较新 clock 不错误阻断实际目标。
- 任一失败完整回滚、失败不返回 ID、输入/输出深复制隔离、16 个并发重复命令只有一个成功，并发 Snapshot race 通过。
- 真实命令构造 unfinished 源→completed 后继后，直接废弃已被引用的历史源会触发现有 `invalidTraceContinuation`，候选整图回滚。未改变 Swift 或 Go 拓扑要求，不把该组合宣称为本批可成功发布的路径。

新文件函数覆盖：AbandonChain 100%、ReactivateAbandonedChain 96.9%、historyChainTarget 83.3%、historyChainActiveTrace/historyChainClockValid 100%。未覆盖的 reactivation nil-settledAt 防御分支、命令层 recurring 防御、缺失 parent 分支均被有效图构造器前置限制，不能绕过 validator 来填满覆盖率。构造器拒绝 nil settledAt、非 pending pin、duplicate pending、recurring 的测试保留。

## 不可声称的内容

没有运行 Swift oracle 或 Swift 测试；仅为源码驱动的 Go 验证。没有真实 macOS/Windows/ARM64、UI/IME/读屏、生产数据或远端账号验证。没有分类捕获实现、Service 新入口、磁盘持久化接通、历史菜单、pooled removal、undo/redo 或完整 App 行为。

后续 Service 必须精确区分 abandon 的实际目标 capture、unfinished reactivate 的新增 capture 与 pending reactivate 的旧事件/latest 原样保留，并验证完整事务。当前 abandoned audit 和非空 catalog 继续 fail-closed。本批只可作为严格隔离纯核心候选整合，不能直接用成功核心图替换完整可持久化快照。

## 正式组合验收（2026-11-06 UTC）

本节是后续正式副本的实际结果，覆盖本批、保存状态文案、年度R2与普通任务链纯核心的组合；前文候选阶段记录仍保留其原来范围。全部467个源码文件在完整检查、构建及CLI期间哈希/模式保持不变。

- 完整 `scripts/check` 退出0：应用主包race470.810s、taskapp101.273s、taskview1.052s、titlesession1.411s、domain core9.821s；四模块race/vet、固定Unicode16及51项依赖检查、MyGo输入/弹层定向门禁通过。inputdraft、titlecoordinator、textcanon、codec canonical为缓存结果，不称为新跑。
- `scripts/cross-build` 六目标全部退出0：macOS/Windows/Linux的amd64/arm64，只是编译，不是这些系统实机验收。主机构建退出0。
- 实际隔离Linux CLI创建、重开、day/pool渲染、会话复用拒绝及隐式scope拒绝均通过；没有打开真实桌面窗口。
- 证据保存在外置BUILD的 `combined-validation.json` 与 `combined-*.log/.exit`。GitHub写入仍待确认，未推送、没有此组合的新hosted CI；462d6da的旧候选发布复现不能套用到本批。完整产品、Swift实跑、系统IME/读屏及真实三平台仍未验收。
