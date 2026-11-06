# Ordinary history 纯模块独立审查

日期：2026-11-06。task-id：review-history-audit-helpers。

## 结论

**Accept 冻结 revision1，仅限尚未接线的三个 package 内部纯 helper。无未决产品缺陷或阻断项。**

本次实际验证支持：固定 forward-clock ordinary profile 的完整 snapshot inspector、确定性 classification-only append builder、两个独立有效端点之间的旧 stream/day-lock 前缀检查。现有 reader、Service、store、codec audit callback、UI、会话格式均未因此改变。

不能由此宣称 Service abandon/reactivate 已实现、历史已获写权限、整个 aggregate 可达、跨事务历史真实、v2 可继续使用、v3 writer 已上线，或 Swift/非 Linux 实机对等。GitHub/远端动作不在此次审查范围。

## 1. 冻结对象与独立性

- 正式只读基线：`b8857778b990fd30dae35c24b03fb49c08c0cf12`；开始时已核对 clean。
- 上游只读基线：`43e6eefaae3a04be89cab3e234d470111afd80ef`。
- R3 设计 SHA-256：`e45ec63d1766dd6c74f1826a500333778cc8022797b7aa271a8b2618377839a3`。同时阅读上游 AGENTS、ports/mygo/AGENTS、R2 独立审查及 R3 勘误。
- 独立目录：`/workspace/scratch/586e77bd59a6/noonmark-mygo-work/noonmark-go-history-audit-independent`。
- 外置 BUILD/证据：本报告所在目录。没有复制工具链或 cache，没有改作者/正式源码，没有运行任何远端写入。
- 收到冻结哈希之前只阅读和准备独立测试，没有编译活动实现。收到冻结后只复制三个实现文件，未复制作者测试。
- 最终对独立副本全部 473 个原有 tracked 文件逐字比较基线 Git objects：全部不变；只新增三个实现和两个独立测试。见 `evidence/INDEPENDENT-SOURCE-DELTA.json`。

冻结文件如下，路径均相对 `ports/mygo/internal/taskapp`：

| 文件 | SHA-256 |
| --- | --- |
| history_vnext.go | d59969205f42ac832a8824fb0cb2266869c43eeaf28f867d8565879bd16ecd86 |
| history_vnext_builder.go | bc3713653051869c918f666407da75b96a3c3096d7b406975c29ac59a8caa64d |
| history_vnext_machine.go | 44ce117f37f7705c5ece5380b8cfe88c429b18b3868266cd9f7a8e21f5be8715 |
| history_vnext_independent_test.go | 0cc208feaef690b5142adf50e57d303b38e1e38d9a62898a6298f001fbc787c4 |
| history_vnext_independent_oracle_test.go | a59dca021faf283df28271a786484c2797a91f346aa2d606a7d17a18c3403e2d |

## 2. 代码层审查

### Inspector 与保证范围

- 先由已有 bounded Marshal → v7.Inspect 取得独立完整对象，再检验完整 runtime/supplement scope。没有先递归 clone 未受限 caller AST，也没有接入现有 Encode/Decode audit callback 导致循环调用。
- 空 catalog 所有相关数组、neutral preferences、空 ideas/series、非 recurrence scope 继续 fail-closed；graph/schema/引用约束由既有完整验证保留。
- 全局 UUID 按大小写规范化检查唯一；stream/latest/key 对齐；revision 恰覆盖 1…N；每流原顺序递增；全局 capture 用数值时序，复制关系使用原始 bits。
- `[A@10], L=5` 被拒是本 profile 不支持的来源输出，不是证明 purecore 错误。open-U 明确只有 source-supported synthetic read extension 的意义。
- 实际 completedAt 的 final anchor 不会被任意中间 completion witness 替代。没有增加 `contentUpdatedAt == completedAt` 的通用公理。

### 有限状态与 token

- 7×2×3×2 的固定数组覆盖 0…83 槽；没有按历史路径分配候选，也没有依赖 wall-time、重试或路径截断来接受。
- 在固定 event index，最近 A 和 L 的 bits 都是固定输入；来源标签不因数值相等而合并。相同完整键保留较小的数值 lower bound，后继只使用单调下界及固定事件/lock/completion 时刻，符合 R3 的支配关系。
- final-completion anchor 绑定实际 completedAt；anchored C 不可 undo。C/R/D event 之后没有其它 capture 边；UAA/UAC/UAD/UAR 必须经完整锁状态而非邻接规则拒绝。
- R/D/X 的 not-applicable token 不代表实际 settledAt=nil。实际 settled/content/capture 的直接 bit 校验与 terminal 时序下界仍然保留。
- ε 图最多84个槽。同键循环不能降低下界，简单有效路径最多83条边，固定点扫描第84轮足以确认稳定；实现的超界拒绝不是正常路径的任意截断。

### Append / successor / 可变数据隔离

- 零值 proof/candidate 被拒。有效 proof 不等于操作 capability。
- Builder 不生成 UUID、不猜 after graph、不把 classificationCandidate 当完整 snapshot；旧 stream 只尾追加，旧 latest key 原位替换，新 key 只插入一次，零 intents 不消费 revision。完整 after 必须另行 inspect。
- Builder 做全请求 ID/status/finite clock/数量预检，checked increment；复制一次旧分类，按索引追加。后续 canonical Marshal 同时限制 nodes/depth/bytes。
- Successor 分别要求两个固定 profile proof，检查旧 stream canonical 前缀、revision 增量、原 day identity/date 和旧非 nil lock bits，不冻结任意 graph 字段，也不批准 graph delta。
- 返回 snapshot/classification copies 不泄漏 map/slice/指针。16个并发读者反复 copy、append、prefix 检查，以及改动返回副本，race 未发现共享写。

## 3. 独立测试覆盖

最终两个独立文件共19个顶层测试；它们的来源正例由 baseline purecore 命令构造，classification captures 在测试中独立投影，不借候选 builder 制造 inspector 的正例。

覆盖包括：

- 空 P/C、单 C/U/R/D、future X、AA、P/latest A、open C/latest A、A→P→C→lock、complete/undo/redo、锁前/后 A、锁后 U/A/U。
- open A→U 与更晚锁日；保留 settled 来源；未锁日 U→A→P 等明确合成扩展。
- R2 completedAt=5/content=15 对 `[A10,C20]` 的直接拒绝，与 complete15 正例；open C 同类反例；C.content17/completed15 的 open/locked 必要条件正例。
- UU、UAA/UAC/UAD/UAR、C/R/D 后事件；terminal no-A/no-L 与后续锁日；X 后续有同日 companion 触发的真实 core lock，且 X 不新增 capture。
- min/max date；最大日没有合法 Today>date；最小日 X 不得借等日 cancellation 伪造 earlier Today。
- 24组 AUL/ALU/LAU 的正负零来源排列；R/D/X settled/content bits；负数、小数、子正规、MaxFloat64、NaN/Infinity、倒退 capture。
- 跨流 UUID 大小写冲突、revision 重复/缺口/count、错误 key/reference/latest、空流、非法 status/category、preferences scope。
- `Ux@10, Az@10, Uy@10` 的必要条件正例，明确不声称 Service 原子批次可达。
- 完整 before → 临时 after+旧分类被拒 → append → 完整 after 再验证的分阶段流程；新 stream 批次、已有 stream 替换 latest、确定性、零 intents、失败不变、未知 trace 候选不能通过 after inspector。
- 两个分别有效端点的旧 UUID/capture 改写、删旧 event 后重编号、删 key、改变/移除 lock、改变 day ID/date，全都被 prefix checker 拒绝；无关 title 变化不被误当 prefix 权限控制。
- 旧 ValidateEmptyHistory 对合法单 capture payload 仍接受，对 A/多事件/open-U 仍拒绝。此数据兼容测试不代表 v2 会话可续写。

### 独立 BFS 的精确保证

**先经候选实现 `historyVNextFinal` 的 final-field 检查筛选，再对1,449个短历史的扫描 NFA 结果作独立 concrete-state BFS 对账，全部一致。**

BFS 保存具体 status、locked、event index、完整 clock/content/settled/completed 值，不使用84槽、token、anchor bit、lower-bound 支配或候选 closure。枚举由有限具体状态集收敛，不靠超时或路径数截断。

它仅独立对账扫描 NFA，不是整个 inspector/fullgraph/Swift oracle，也没有独立证明作为前置筛选的 `historyVNextFinal`。final-field 的关键保证由上述 core 来源正例、直接 completedAt 反例及一因素篡改另行覆盖。

## 4. 实际资源边界

所有边界均实际执行，没有用超时充当正确性结果。

- 完整合法 envelope 恰16,777,216 bytes 通过 inspector；多1 byte拒绝。
- caller AST 恰1,000,000逻辑节点可 Marshal；1,000,001命中 node-limit；循环和超深 AST拒绝。节点测试使用共享不可变叶数组节省物理内存，但计数按逻辑出现次数，未绕过 writer 的节点计数。
- 完整4,096-event source-supported A/P循环 snapshot 通过；状态/闭包实测最大3/3。
- 机器分层扫描66,666 events（1,000,000/15的保守计数边界）通过，slots/closure最大3/3；builder 66,667 intents 在数量预检拒绝。此66,666不是声称完整 wire 能装下该数量。
- 固定A循环形状的最大完整事件包另用外置 Go overlay 探针实际验证：**53,285 events、16,777,023 bytes，余193 bytes，通过完整 inspector；53,286 events 命中 byte limit**。同一路 canonical Marshal 同时执行 node/depth上限，不是只考虑 bytes。该形状事件节点约799,275加固定图/索引，低于1M；不把53,285宣称为所有可能 snapshot 的统一最大事件数。
- 最大完整事件探针单轮运行，没有多轮大AST二分；只以1/2事件长度求增长公式，并由实际完整正例字节数复核公式。
- 槽编码完整遍历0…83，84个key一一对应；计数器实际可表示84槽。

## 5. 命令与结果

环境：Go1.27.3 linux/amd64，既有 GOCACHE，GOPATH `/tmp/noonmark-gopath`，GOPROXY/GOSUMDB=off，GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB、`-p=1`。使用 `scripts/environment` 及外置BUILD准备固定六patch MyGo。没有冷cache多目标构建、release、Swift oracle、实机/UI或真实数据操作。

最终运行记录：

| 检查 | 实际结果 | 日志 |
| --- | --- | --- |
| 独立 focused 扩展矩阵（加入最后的批次/槽测试前） | exit0，package2.411s | logs/independent-focused-r1-final.log |
| 冻结19测试全部 + 旧 EmptyHistory/Aggregate，含完整资源边界，race | exit0，package24.775s | logs/independent-race-r1-final.log |
| 冻结树 taskapp vet | exit0 | logs/independent-vet-r1-final.log |
| 冻结树全部 taskapp 测试 | exit0，package22.868s | logs/independent-taskapp-r1-final.log |
| 外置 overlay 最大完整事件探针 | exit0，package4.098s | logs/independent-maximum-full-events-r1.log |

每个最终 gate 保存 `.exit`；race/vet/taskapp 另保存 `.command`。最终全量 taskapp 包含已冻结全部19个独立测试；没有把更早的focused记录冒充最终同树证据。

观察到的最大单子进程RSS：最终race349,868KiB、全taskapp112,704KiB、最大事件包273,636KiB。这是 `resource.RUSAGE_CHILDREN.ru_maxrss`，不是多个同时运行进程的内存总和，也不是平台级硬保证。

复跑入口：`run-independent-validation.sh`。最大事件探针实际执行的源码/overlay分别为 `evidence/maximum-events-overlay-test.go`、`evidence/maximum-events-overlay.json`，精确命令/exit见 `logs/independent-maximum-full-events-r1.command` 和同名 `.exit`；其hash全部在 `SHA256SUMS`。

便于随任务备份的最小探针：`maximum_full_event_envelope_probe.go.txt`。它仅含额外测试函数，依赖冻结 `history_vnext_independent_test.go` 的 iaPath/iaNow/iaWire/iaObj/iaNumber/iaMust/iaReject 与该文件已有 imports，以及本任务三个实现和 baseline core/codec。无需作者测试。`reproduce-maximum-full-event-envelope` 读取指定端口源码、核对冻结测试SHA，生成外置 overlay后执行同一命令，不修改真实源文件，也不加入默认测试入口。调用方式与所需环境见 `PROBE-REPRODUCTION.md`。

## 6. 真实失败与修正记录

### 审查测试过强，不是产品修复

首轮 `logs/independent-focused-r1.log` exit1，有4条 signed-zero 负例失败；其余矩阵及1,449例BFS已通过。原断言/日志保留，见 `evidence/PREFIXTURE-RED-history_vnext_independent_test.go`。

四种情况为 AUL/ALU，A.bits 与 U.bits 相反、U.bits=L.bits，所有时钟数值都为零。原测试把 final settled 改为 L.bits 后一概要求拒绝，错误地把“用于造 fixture 的那条 reactivation 路径”当成唯一可能来源。

R3允许另一完整解释：A → reactive P → Lock(P→U)，此时 event U、settled 和 L 可以 bit-exact 相等。因此只在这个完整备选路径成立时改为通过断言；无此备选路径（含LAU已消费锁）的 bit 篡改拒绝断言保持。没有修改任何实现文件，也没有放宽产品行为来消红。

### 测量工具缺失

一次 harness尝试因 `/usr/bin/time` 不存在而exit127，未运行Go gate。原记录保留在 `logs/harness-missing-usr-bin-time.*`。改用标准库 Python subprocess/resource 记录时间和退出码后重新执行，最终日志如上。不是产品测试失败或权限拒绝绕行。

## 7. 交付边界

批准整合这三个冻结实现和两个冻结独立测试，但仍仅为未接线纯模块。正式聚合检查由整合后的精确树另行执行，本报告不替代它，也不将其它运行报告当成本次执行证据。

后续 typed replay、统一 mutation frontier、Service/runtime接线、新v3 clean-cut、磁盘故障/并发/重开、UI及Swift差分均未被本报告批准为已完成。没有production/真实会话/home/iCloud/Keychain/真实AI访问或GitHub写入。

## 整合后的补记

以上是独立冻结树审查记录。其后6个实现/测试文件逐字合入正式候选，完整聚合、六目标构建、Linux构建与隔离CLI均exit0，详见VERIFY.md。便携容量包装随后实际运行也exit0（package4.051s）；未改3个实现或2个独立测试。它仍不是Service/store/UI接线或Swift/实机验收。
