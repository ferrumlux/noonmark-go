# 子任务 pure history helper 独立验收

2026-11-06。裁决：本次独立对抗测试通过，未发现 production 缺陷；仅接受下列明确覆盖的 pure helper 范围，不是全面移植对等或接线准入结论。

## 独立性与身份

独立 clone：`../subtask-history-independent-acceptance`，基线 `e35a5d5d628b7c0d0a2ece945fc18e944d344ee0`。只从冻结候选移入三份 production 文件；没有读取、复制或运行作者本批新增测试。测试为本次新写 `ports/mygo/internal/taskapp/acceptance_subtask_independent_test.go`。主要 pool/actual fixture 自行以 CreatePoolTask、EditPlannedSubtask、ScheduleFromPool 等领域命令创建；alias 回归使用正式基线既有 `reviewUndoContinuationAlias`，明确属于参数拓扑，不冒称可达 Service 状态。

`IDENTITY.txt` 记录三份候选 production SHA256、测试 SHA256、clone 状态。三文件均逐字节匹配委派的 PRODUCTION-SHA256.txt。正式基线身份、clean 与旧 command hash 见 BASELINE-AND-RESULTS.txt。旧 command 自 auditSettlementHistoryVNext 至 EOF 与 HEAD 逐字节相同，suffix SHA256 为 ac1d9c4a57753c20a3a906bbad64e961cd92bd115304528ccb7e2e8a8854b17d。

曾用 go fmt 命令格式化本独立 clone 的 taskapp 包，只有 command 原有对齐发生变化；已立即从冻结源恢复，最终 focused/race 均在精确三文件 hash 上重跑。没有改动正式／作者工作树。仅本独立 clone 和 external BUILD 有写入。

## 实际执行

外置 BUILD 为本目录，HOME 为本目录/home，GOCACHE 为本目录/go-cache，只共享 module cache。先 mkdir BUILD，再 source build-environment/activate.sh，工具链为 Go1.27.3；没有使用 /usr/bin/go。无网络、GitHub 写入、CI、Service、真实资料、AI、iCloud、Keychain 操作。

在独立 clone 的 ports/mygo 下：

- `$NOONMARK_GO test ./internal/taskapp -run '^TestAcceptance' -count=1 -v`：exit 0，11 个 top-level tests 全通过，见 focused-final.log、focused-final.exit。
- `$NOONMARK_GO test -race ./internal/taskapp -run '^TestAcceptance' -count=1 -v`：exit 0，全部独立新增测试通过，见 race-final.log、race-final.exit。

这是 pure taskapp 包定向运行，不是 MyGo GUI build、全 scripts/check 或全部项目测试。早期 fixture 构建错误（planned child CreatedAt 早于链、ReturnToPool 缺 PlannedIDs、父任务仍有 open children 而尝试完成）已按领域契约修正；不是发现后改 production，也没有降低断言或 skip。

## 具体覆盖与发现

1. **planned 4 动作**：三项池任务、中文 CRLF/CR 标题归一化常量、精确字段 delta、两个 parent clocks、lineage/ID/position 保持、物理 remove。额外参数例将数组次序与 position 次序错开，确认 remove 以数组次序重排而非 position 排序。add 独立断言新增项关键字段，其他所有领域字段与独立构造 expected 做 DeepEqual。
2. **actual 7 动作与旧历史**：每动作从两次 A→P typed 历史起步；精确保留两个分类事件及整段 canonical 分类，count=2。逐字段独立期望 title、difficulty、delete tombstone、complete、undo、abandon；父 chain/trace/definition 全部不变。delete 不物理删除，后续 add position 包括墓碑，重复命中 X 即 NotFound。
3. **恢复 witness**：合法带 cancellation witness 的 child 上，title/difficulty/complete/undo/abandon 保留，delete 只以 fresh witness 替换。取消 ID 与旧 witness 碰撞早于不存在 owner。
4. **no-op / clocks**：planned 和 actual Unicode canonical equivalent title 携带早时钟、NaN、正负 Inf、MaxFloat 允许，完整 canonical bytes 和 raw Now bits 保留。planned 真变化覆盖 -0→+0、+0→-0 与负数 equal/后继组合，检查写入与冻结字段 exact bits。
5. **guard / shape**：wrong owner 早于日期、past 早于 no-op、shape 早于 owner、missing trace 早于 clock、invalid consumed UTF8 title 早于 owner；actual 各动作所有不允许的 Lineage/Title/Difficulty/Cancellation 字段逐一投毒。空／unknown／跨 family kind 拒绝；future P 三状态动作 ImmutableHistory、future 内容可编辑；已完成 parent 的 add InvalidTransition、title ImmutableHistory。每负例指定 error 并断言零 proof。
6. **预算 / 10 kind 闭集**：穷举 uint8 0..255，只 1..10 valid。planned 六、actual 七 string 字段逐一 exact MaxBytes / MaxBytes+1；正常必填字段累计边界、shared backing 逻辑累计、unused CancellationID 超额。seal→before→after→input→bind 顺序实测；AST 独立检查两个入口前四条 iff 调用全集及紧接 bind，禁止这些 gate 前藏额外调用。预算文件静态确认只有 len/raw 累计，不提前 UUID/UTF8/normalization/Clone/Validate 扫描。
7. **ownership / 直接 fence**：planned after nested description pointer、before planned slice、返回 canonical copy；actual before slice、after completedAt pointer、返回 copy 全部故意改写，结果再次 snapshotCopy 不变。每动作直接调用专属 fence，拒绝合法 sealed final 中无关 parent title 或 chain clock 改动，不能只凭 match 拒绝冒称 fence 覆盖。
8. **historical full-definition fence 实证**：原 baseline undo-continuation alias 在 seal 精确 historyToday 拒绝。改 X cancellation date 为前一天并设置合法 existing planned child 后，prior、四动作 core final 都 full inspector 通过，bind/match/finish 逐层通过，然后四动作 audit 精确 historyCommandFence 拒绝且零 proof；直接 fence 也拒绝。历史 definition 的 clock-only 变动被拒；别名上 title/difficulty NaN 真 no-op 被允许。真正 ReturnToPool 建 fresh definition 的后续编辑通过，旧历史 definition 字节不变。

## 可信范围与明确未覆盖

没有发现本次冻结三文件的行为或预算顺序缺陷。证据强于仅重跑作者用例：新增测试未参考作者新增测试，关键期望来自独立常量、完整 field delta、错误枚举与原始 canonical 比较，而非只将同一 core 函数与自身对照。

本次不是完整 TEST-MATRIX 笛卡尔积。未穷尽所有异链历史 [A,U]/[U,A,U] 与所有 locked/status/date 组合；未跑 R/current-definition 第二种 alias 拓扑；未遍历所有合法 child status、所有 unused shape 的 planned 交叉、所有 graph budget overflow 构形、全部 canonical unknown field/分类前缀污染、深度和 escaped-byte 边界。这里的 race 只证明这些执行未报告 race，不支持审计期间 caller 并发修改输入（契约明禁）。未做 Swift oracle、macOS/Windows、真实 UI、store reopening/durability、writer/frontier、年度容量或生产对等验收。整合仍须实施团队既有全部 fast gates 及独审通过，不能因为此报告跳过。
