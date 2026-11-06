# GO-SNAPSHOT-CI-001：Snapshot probe 的默认 spec 路径

状态：原缺陷已重现，候选本地验证已通过；新提交的 hosted development CI 待复验，不代表完整 I1 已通过。首次发现：2026-11-06 UTC。

## 影响与归因

真实开发分支 [run 42287483351 / attempt 1](https://github.com/ferrumlux/noonmark-go/actions/runs/42287483351/attempts/1)，head `d9ebb4860ad3d834e5737ffb501f30726ae54569`，tree `295c114aeb63d990d715db42595682a9f21a4f9c`，job `128131960056`，于 2026-11-06T20:33:49Z 以 failure 结束。第一模块执行 `go test -p=1 -race -buildvcs=false ./...`，递归发现新增的 `scripts/swift-snapshot-oracle/go-probe` 包；`TestActualIngressMatrixFaultControls` 在 main_test.go:282 报 `lstat r2-seeds/planned-12.json: no such file or directory`。

根应用包实际用时 471.006 秒，低于未改变的默认 600 秒 timeout，且该包已通过。失败的是随后 probe 包，不能把它误诊为根应用超时，也不能称整个根模块通过。root vet、其余三个模块 race/vet、Unicode/dependency/native 定向门禁、六目标编译及合成 CLI/headless guards 均未到达。

已确认的失败源码检查点为 d9ebb486（unfinished Snapshot source observer）；精确首个引入提交尚未单独归因，缺陷所在测试已与该 head 的 Git blob 核对。本记录不从 Git 身份推断自然人作者。没有生产资料受损或产品运行入口变化。

## 根因与最小修正

冻结的 `spec/r2-seeds/planned-12.json` 实际存在。此测试单独直接读取 `os.Getenv("SNAPSHOT_SPEC")`，环境变量未设置时得到空串，拼接后的路径错误地相对于 Go 包目录寻找 `r2-seeds`。同一文件已有 `testSpecPath()`，非空环境值优先，否则返回 `../spec`；其他依赖 fixture 的测试已使用它。

修复统一调用该 helper，在原测试内新增默认未设置与显式有效 spec 路径的子测试回归，并在这两种环境下运行完整 probe race suite。不跳过或删除断言，不复制或生成替代 fixture；normal、bypass、late 三种 ingress 控制及 294 行矩阵、18 cases、32 points 的语义不变。冻结 spec、Swift 来源与 catalog 输入不得为迁就测试而改写。

## 原始失败证据

外置原始审计目录为 `snapshot-branch-hosted-ci-42287483351`，保留官方 run/job/steps/source tree、exact-head source receipts、完整日志与下载产物。重要校验值：

- 完整 job.log：37038 bytes / 449 lines，SHA256 `4ca19ffc81c1b4d81cd02f52fc6c78c875db2a3f66a68ff463130870acdbae4c`。
- Artifact `13538318461`：928 bytes，SHA256 `e3f07fc989413f011fcb97a9bd20bc7ff96e1f2f540819614cd7938d226e21be`，ZIP CRC 已核验。
- `checks.txt`：943 bytes，SHA256 `5a9a2e151b6dc84ba7430f3499b7494b41d722484e57b59bf3df8a63e504fc15`。
- `environment.txt`：416 bytes，SHA256 `f1d7714ccfaf61aee9e969304a81a3c9521fadc2b781e8153dcca9238cde4d1d`。

Artifact 只有上述两个成员；`binary-checksums.txt`、`cross-build.txt`、`entrypoints.txt`、`result.txt` 不存在，不能记成六成员成功产物。原始审计核对了 17 个 exact-head source files，并独立核对失败测试源码。

## 本地红例与适用门禁

候选基于相同 d9ebb486。在任何修改前，从 go-probe 工作目录执行 `env -u SNAPSHOT_SPEC go test -run '^TestActualIngressMatrixFaultControls$' -count=1 -v .`，退出 1，同样在 main_test.go:282 报缺少 r2-seeds/planned-12.json。原始日志为外置 `snapshot-ci-fix-build/red-unset.log`，SHA256 `9c0767c80e33e40961df3d77a82c1c2d0720d5e0bb236ba5848aff80ffd9ea60`。

此缺陷属于 `scripts/check` 的 Go 包递归执行范围，适用的 fast/symptom 门禁均为 `TestActualIngressMatrixFaultControls`，在 `gates.tsv` 登记；同一个测试的 unset/explicit 子测试各运行三种模式并保留所有原断言。既有 registry 校验须包含该 probe 测试目录，不能往登记表填写不存在或未受聚合覆盖的测试。冻结输入 hash、tool-manifest、150 项 Python 合同及 source 容量/分块常量的适用复核由本次最终验证记录提供；未经运行不得称通过。


登记与工作流的局部验证已完成，均退出 0：

- `go test -count=1 -v failure_gates_test.go`：TestFailureGateRegistry 通过（0.079s）；外置 `failure-gate-registry.log`，SHA256 `61cd90f1de3b758e2790076e8e5c125ac4d7ffe2f976fe8f88d350acb1d2d6ca`。此命令只验证登记与源码合同，不冒称完整根包 race suite。
- `python3 -B scripts/ci/validate.py`：8 项合成 runner fixtures、9 项 workflow 安全拒绝 fixtures、shell syntax、有界同步日志及 write-failure fixtures 全部通过；外置 `ci-validate.log`，SHA256 `224f72fa7f84b7e2bba17bb79b041e6104cfcf2d6ac8a02082e58daa299ca3ee`。此命令不是实际 GitHub runner 或完整业务执行。

## 最终本地绿例（2026-11-06 UTC）

基于 d9ebb486 的独立候选完成以下实测；没有重跑 root 完整 aggregate，也没有把旧 CI 失败改写成通过：

- 完整 Go probe `-race -count=1 -v`：外层环境 unset 67.816s、显式有效绝对 spec 路径 64.511s，均退出 0。每轮包含全部原测试及矩阵 unset/explicit 两个子测试；每子测试保留 normal/bypass/late 三模式、294 行断言。
- 原 150 项 Python 合同全部通过，165.666s；包括未缩短、未跳过的真实 120 秒 stall / kill / reap 控制。
- 当前工具 manifest 45 项全部核验，只更新 main_test.go 的 hash/bytes；14 份 spec 与 d9ebb486 逐字一致，19 Swift source manifest 及 789 catalog children 不变。
- 7 份 probe 源码由 62811 增至 63157 bytes（+346），按原合同独立留存 7 件；139 产品路径、133 独立 OID、9462103 bytes、196 非空 chunks 不变。总输入 9525260 bytes，10MiB 上限余 960500；无需改 source-policy 或局部计数断言。
- 重新认证真实 Git source 原件并运行真实 batch、离线复算与伪造／缺失负例，9 项通过。当前新增原件及收据 255 件、9721400 bytes；加入固定依赖原件后 313 件、17059590 bytes。收据路径和时序字节为本次实测，不把相对旧证据的变化全部归给源码增量。
- 重新执行标明容量用途的饱和 fixture：8192 件、正常总预算 77589126 bytes、正常余量 5248378 bytes，实际写出 tar 41492480 bytes 并逐成员及 manifest 引用复核。此处填充只用于原有饱和容量负荷，不改源码、业务 fixture 或计数以凑旧值；不代表完整 I1 执行。

外置 `snapshot-ci-fix-build` 最终日志 SHA256：

- `green-unset-race.log`：`4270c172ebbea6783c3c87b1dc377a86664257ca1e1f0f88f87a3f4df767fcdb`。
- `green-explicit-race.log`：`9bbf19da1af851f11619d2cc1ba17cda28cec711c179350852c0d01bcc8c88e2`。
- `python-150.log`：`a92247bd31da7ef5f6180a3101b213e682a8e37b04bc2166e32318405658dd2d`。
- `source-real.log`：`328fbbe72168efbec9778934160aca1b00f84d199feb37f46264174976d6c2b3`。
- `manifest-frozen-capacity.json`：`b4a931b04f44cf78777390b967381c7c241e7a82dbc004a5277a7670ea416183`。
- `CAPACITY-RESULT.json`：`d4745c847c5ac9bd896ef9f85dd51c25ef945357425a8da5442d479b0bba58dd`。

## 限制与回滚

本轮不重跑约 17 分钟的 root 完整 aggregate；新候选须在精确提交上重跑实际 hosted CI，并审计完整阶段和六成员产物。此次 Go 测试修正不是 Swift 实际执行、完整 I1 binding、六主机运行、签名、发行或产品全面对等的证据。`runtimeAdmission` 仍为 false，生产入口仍拒绝。

仅回退此测试路径修正和对应登记/记录，不改业务数据或冻结输入。若后续 CI 仍失败，保留本次真实失败历史，按精确失败阶段继续定位，不提高旧 timeout 或降低 gate 断言。
