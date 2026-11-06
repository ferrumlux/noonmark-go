# 验证记录：普通任务历史候选 r1

日期：2026-11-06。本文件仅记录本实施者实际执行的命令；独立验证另以其冻结报告为准。

## 结果与边界

**本候选已通过合成聚焦测试、完整 taskapp 回归、重点 race 与 vet。没有接线、没有 Swift oracle、没有非 Linux 实机验收。**

正式仓库保持 `b8857778b990fd30dae35c24b03fb49c08c0cf12` / clean；候选仅多 4 个 Go 文件，所有旧文件逐字相同。`git diff --no-index` 生成可审查 patch，但没有 commit、push、GitHub 写入或远端调用。

本批没有 production、真实用户会话、home 扫描、iCloud、Keychain、真实 AI 或新运行会话。测试均使用固定 identity 的合成任务资料；旧 taskapp suite 自有隔离临时目录跟随显式外置 TMPDIR。没有复制工具链/缓存或清理共享缓存。

## 固定环境

- Go：`/workspace/scratch/586e77bd59a6/noonmark-mygo-work/go/bin/go`，1.27.3。
- BUILD：`/workspace/shared/noonmark-history-audit-vnext-20261106`。
- GOCACHE：`/workspace/shared/noonmark-go-build-cache-20261105`，复用不清理。
- GOPATH：`/tmp/noonmark-gopath`；GOPROXY=off，GOSUMDB=off。
- GOMAXPROCS=2，GOGC=20，GOMEMLIMIT=700MiB；所有 Go 命令 `-p=1`。
- 通过候选的 `ports/mygo/scripts/environment` 准备固定六 patch MyGo、外置 GOWORK/dependency 与 TMPDIR。未跑冷 cache 六目标或 release。

每条测试前使用：

```sh
export NOONMARK_GO=/workspace/scratch/586e77bd59a6/noonmark-mygo-work/go/bin/go
export NOONMARK_MYGO_BUILD_ROOT=/workspace/shared/noonmark-history-audit-vnext-20261106
export GOCACHE=/workspace/shared/noonmark-go-build-cache-20261105
export GOPATH=/tmp/noonmark-gopath GOPROXY=off GOSUMDB=off
source ports/mygo/scripts/environment
```

脚本把工作目录切至 `ports/mygo`，以下命令均在该目录执行。起止时刻为 UTC。

## 实际命令及退出码

| 阶段 | 命令 | 起止 | 结果 |
| --- | --- | --- | --- |
| 初稿编译 | `$GO test -p=1 ./internal/taskapp -run '^TestHistoryVNext' -count=1`（当时尚无新增测试） | 首轮实现写入后 | exit 0，no tests to run，仅编译证据 |
| 初次测试编译 | `$GO test -p=1 ./internal/taskapp -run '^TestHistoryVNext' -count=1 -v` | 02:43:41Z–02:43:41Z | exit 1，两个测试的 map value 字段不能直接赋值；未运行断言 |
| 修正测试编译后聚焦 | 同上 | 02:43:56Z–02:43:59Z | exit 0，package 0.255s |
| 全 taskapp | `$GO test -p=1 ./internal/taskapp -count=1` | 02:45:10Z–02:45:31Z | exit 0，package 19.942s |
| 重点 race | `$GO test -p=1 -race ./internal/taskapp -run 'TestHistoryVNext\|TestEmptyHistory\|TestAggregate' -count=1` | 02:45:31Z–02:45:39Z | exit 0，package 3.622s |
| vet | `$GO vet -p=1 ./internal/taskapp` | 02:45:39Z–02:45:40Z | exit 0，无输出 |

初次编译红日志未覆盖，保存在 `logs/focused-initial.log` 和 `.status`。通过日志为 `logs/focused-1.log`、`taskapp.log`、`race.log`、`vet.log` 及对应 `.status`。没有把编译成功、无测试运行写成语义验收。

## 本实施者合成矩阵

`TestHistoryVNextCoreCommandProjectionMatrix` 通过真实 purecore 命令造 task graph，再由独立于 builder 的显式 projection fixture 构造 capture；不是 Service/writer 输出。

- 旧 narrow CaptureEmptyHistory 的真实一次 C/U payload；空 P/C；无历史 future X；单 R/D。
- A/P/A、多次 locked U/A/U、P/latest A、open C/latest A、A/P/C/lock、complete/undo 后 final anchor、A/P/R、A/P/D。
- A 后较晚锁日、先 U settlement 再 A；R/D 后锁日。
- 明确标识的 source-supported synthetic open U、open U 后较晚 silent lock、open 多次 U、相等时间多分支。

负例是直接 mutation 或 profile 反例，断言必须拒绝：

- UAA/UAC/UAD/UAR、UU、C/R/D 后再 A，不能只靠邻接表放行。
- purecore 实际允许 A@10→Lock@5，候选按 unsupported profile 拒绝，不称源无效。
- open/locked C 的实际 completedAt 被改为末 A 之前，必须返回 completedAt witness 错误。
- retained settledAt 错源；+0/-0 赋值来源不同；X 带旧 A 历史不得删除修复。
- 最大日期 lock/open U 拒绝，最小日期 open U 与 lock 正例、最小日期 future X 拒绝；最大日期 A/P/未锁 C 正例。
- 全局 UUID 大小写冲突、重复/缺口/巨大 revision、capture 倒退、key/trace mismatch、stale/missing latest、空 stream、dangling、pending/changed event、NaN/Inf、preferences、unknown supplement、非空 catalog mutation。
- -MaxFloat64、负小数、负子正规、signed zero、正子正规、正小数、MaxFloat64；全部按实际 bits 保留，数值时序不采用 uint64 排序。

builder/ownership/prefix：

- before valid → after+旧分类 invalid → builder candidate → 完整 after inspect valid。
- existing stream 追加、latest 原位替换、零意图无 revision、固定输入确定性、重复预分配 UUID、非法 ID/status/At、意图内重复拒绝。
- 新 trace key 可返回分类候选，但组合 dangling after 必须拒绝，证明 builder 不冒充 aggregate validator。
- 零 proof/candidate 拒绝；输入、snapshotCopy、classificationsCopy 修改不影响内部 proof；失败不修改输入。
- 两端分别 valid 但旧 UUID/clock/事件删去并重编号、day ID 或旧 L 被改，successor 拒绝。
- graph title 的独立合法变化可通过 prefix，明确这不是命令资格授权。
- uint64 checked-add 溢出与精确上限；输入 AST cycle/depth/1,000,001 node 拒绝；过大意图数组在遍历前拒绝；event node 计数含 keys。
- 每个正例检查 stats.MaxSlots≤84、MaxClosureRounds≤84。独立验证另负责更大事件输入及具体 BFS 对账。

旧 reader 门禁独立测试 `TestHistoryVNextOldReaderStillClosed`：A、P/latest A、open U 及多事件候选 inspector 通过后，既有 ValidateEmptyHistory 与 encodeSnapshot 继续拒绝。没有放宽任何旧测试。

## 红绿证据的准确含义

本批新增独立模块，不是已知 production 故障修复。初始 red 是测试代码编译问题，改为重新赋 map value 后 green；未把它宣称为业务 bug 的复现。

语义负例在测试中以“候选必须拒绝”作为 green 条件。上游来源 fixture 与恶意 mutation 有明确注释区分；没有把手工 snapshot 说成真实 Service 生成，没有补造日志或恢复未记录动作。

## 冻结、接线审查与未验证项

`revision1/code.patch` SHA-256：`3eec821cfab1760a2205bb50660f7a0085ab694311f9736cfda25d03354f8ca9`。

4 个源码 SHA 见 `revision1/SHA256SUMS`。本 r1 冻结后不覆盖其实现；如独立审查发现问题，应新建修订证据，保留 r1。

未验证且不能从本批推论：

- Swift 源语义 oracle 与跨平台实机 UI。
- 全 aggregate Service 可达性、真实跨流事务批次、完整命令资格及授权。
- v3 marker/record、旧 binary 拒绝、新会话 writer、Open/AdvanceTo/clock collector 接线。
- 磁盘前缀保护、提交未知结果、故障注入、双 handle 竞争、崩溃恢复与断电持久性。
- production、iCloud、Keychain、真实 AI。

独立验证完成后由整合审查决定下一步。当前交付只能称“未接线纯审计模块通过合成验证”。

## 正式整合验证（2026-11-06）

6个Go文件逐字导入冻结r1与独立测试；旧473个tracked文件除README边界说明外完全不变。正式全量验证期间476个端口文件的bytes/mode保持不变。独立报告与可复现容量探针在聚合结束后附入，没有改动被测试的Go实现或测试。

- scripts/check：exit0。根UI/race466.162s，taskapp124.703s，titlesession1.391s；inputdraft/taskview/titlecoordinator和若干未改领域包使用有效cache，未冒称全部fresh。Unicode16/源码路径门禁、51项dependency测试20.628s、MyGo定向UI2.637s与popup1.044s通过。
- scripts/cross-build：macOS/Windows/Linux × amd64/arm64六目标全部exit0，只代表编译。
- scripts/build：exit0。实际Linux隔离CLI创建、重开、复用/身份守卫、Day/Pool原生headless渲染均exit0；没有实际桌面窗口。
- 归档的便携最大事件探针包装已在正式树实际运行，exit0/package4.051s：53,285 events、16,777,023 bytes通过，下一条超byte limit；slots/closure最大3/3。不是仅bash语法检查。默认门禁未被更改。

所有测试资料均合成、外置隔离；无生产资料、Swift oracle、非Linux实机、新版hosted CI或新版源码绑定release复现。GitHub写操作仍等待工具确认，未重试或绕行。当前仍只是未接线模块，Service/store/UI不能读写新历史事实。
