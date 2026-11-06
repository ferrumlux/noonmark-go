# 修复 Swift oracle 的 package 编译上下文

- task-id：`fix-swift-oracle-package`
- 等级：A
- 日期：2026-11-06（UTC）
- 状态：已修复（独立oracle编译／捕获范围）；e35a5d5的真实macOS症状、同head MyGo CI及1807项有限Go字段对账均成功。当前证据补丁另作本地定向回归，不外推完整Swift／codec／UI或发行。

## 需求与现状

修复真实 [run 42182456188](https://github.com/ferrumlux/noonmark-go/actions/runs/42182456188) 的 `compile-core_failed`，保留上游公开源码、所有输入验证及隔离边界。该 run 的 16 个 Python 合同测试通过，macOS 15.8.2／Xcode 26.2 已进入编译；Swift 明确报告 `package` 声明缺少 `-package-name`。这不是工具链缺失，也不是 Go 对账结果。

原入口用 `swiftc` 独立编译完整 `NoonmarkCore`，没有经过 SwiftPM 注入 package 上下文。修复前的合同测试只覆盖编译前拒绝路径，未检查实际编译命令；本修复新增的默认桥与编译调用合同见下文。

## 设计与边界

1. 先新增离线回归，直接调用实际 `main()`，以显眼的合成源码和模拟 subprocess 检查它发出的编译命令；先记录旧入口红测。这只是 Python 控制流合同，不是 Swift 运行证据。
2. 只在 `compile-core` 命令补固定 `-package-name Noonmark`，对应固定上游 `Package.swift` 的 `name: "Noonmark"`。这是该隔离模块的 package 上下文，不声称复现整个 SwiftPM 构建图或其命令行命名方式。独立 harness 不获得同包身份，仍只能依赖公开 API。
3. 不改 source manifest、上游 commit/tree/blob/hash、向量、harness、workflow、Xcode 或部署目标，不将 `package` 改为 `public`，不增加 fallback 或跳过编译。
4. 比较器严格绑定 runner SHA，因此精确更新 `pins.json` 中此项及 `compare.py` 的 pins 文件 SHA；保留所有比较器校验。
5. 编译失败必须保持失败 summary，并阻止后续 capture；真实症状 gate 仍为现有 macOS workflow 的实际 `run.py`。

## 验证、风险与回滚

离线合同、固定文件 hash 和 Python AST 为本地门禁。需要新 commit 在 macOS hosted CI 实际通过 Core／harness 编译、sandbox 捕获及产物校验后，才可将故障案例改为已修复；Go 对账仍须独立执行。Linux 没有 Swift，不能代替此验收。

仅回退此独立修复提交及配套 runner pin 即可撤回；不涉及应用数据或运行身份。案例与强制门禁映射见 `../../engineering/failure-cases/GO-SWIFT-ORACLE-001.md`。

## 2026-11-06 本地结果

基线为 `c2ca275d8e43975b82f71a633decfe6d0dc88b97`，在独立 clone／分支修复，未修改正式工作树、上游源码或 GitHub 状态。证据保存在源码之外，不将编译产物或运行资料放进仓库。

- 先只添加合同，运行 `python3 -B ports/mygo/scripts/swift-name-oracle/test_oracle.py ContractTests.test_main_core_compiler_has_package_context_harness_stays_external`：exit 1，`AssertionError: 0 != 1`，证实原 `main()` 未传 package flag。红测日志 `01-contract-red.log` SHA256：`e87073bfb07ab8564590913585a7f1a8061ba3ed91634725b14975622119a968`。
- 最终 `python3 -B ports/mygo/scripts/swift-name-oracle/test_oracle.py`：19 项通过（原16项、新增2项编译调用／失败门禁、1项映射校验）。`RUNNER_TEMP` 显式指向外置测试目录，fixture 不进入正式 capture evidence。
- 真实 Go 1.27.3 下运行 `GO="$NOONMARK_GO" NOONMARK_SWIFT_COMPARE_TEST_ROOT="$NOONMARK_MYGO_BUILD_ROOT/comparison" python3 -B ports/mygo/scripts/swift-name-compare/run-tests.py`：28 项通过，26.946秒。这是合成比较器测试，执行真实 Go probe，但没有执行 Swift。日志 `04-comparator-regression.log` SHA256：`9e450469cb5a6f2e51bf28e427af41beba4eee9c5d162eeb7d3c0c6027fa0c4e`。
- `compare.load_inputs()` 严格核对12个来源 pin 通过。新 runner SHA256：`47a6a225f124a80cf3461ba6aad772607ab787dd7f9aaf3f32b5603c8642b3ee`；新 pins 文件 SHA256：`894931b66855c98ff55044509b9c2fa84526163e1ec40c151ed2764ed4fc9c87`。
- workflow、source manifest、vectors、NameOracle.swift 均与基线逐字节相同；读取的上游 Package.swift 与既有 SHA256／Git blob／字节数相符。修改的 Python 可被 AST 解析，`git diff --check` 通过。
- Linux 直接执行 `run.py` 仍 exit 1，`macos_required_not_swift_verified`，不伪造平台或成功捕获。

## 新 commit 的 CI 复验步骤

1. 整合者审核补丁及精确文件 SHA，形成新 commit；核对远端该 commit 和分支实际存在。保留原失败 run 与 log，不能只在旧 commit 重跑后宣称修复有效。
2. 修复合入 main 时，修改 `ports/mygo/scripts/swift-name-oracle/run.py` 会触发已有 push workflow；非 main 分支则手动触发已有 `swift-name-oracle.yml`，选择该精确修复分支。不要改 workflow、Xcode、权限或 artifact 白名单来求绿。
3. 跟踪新 run 到终态，核对 `head_sha` 等于修复 commit，`Test isolated capture contract` 的19项通过，`Capture actual upstream Swift behavior` 和 artifact 上传都成功。编译或 sandbox 后续若失败，保留新日志并继续定位；不把本次离线结果替代失败。
4. 取得本次 run／attempt 的原生 artifact，核对来源、hash、只含限定 JSON。`summary.json` 必须为 `oracle_capture=succeeded`、`stage=complete`，且 capture SHA 匹配；检查108 cases、17 pairs、12实体往返及实际 metadata。`runnerSHA256` 必须等于上述新 runner hash，workflow／source／vectors／harness pins 不变，`workflowCommit`、`runID`、`runAttempt` 与本次运行相符。
5. 捕获成功与有限 Go 对账分别记录。若实际 ICU Unicode 不是16.0.0.0，保持 unsupported，不能改版本标签求绿。真实 capture 通过、相关回归完成并确定修复 commit 后，回填案例和映射状态；在此之前持续标记处理中。

上述本地准备阶段尚未验证实际macOS编译／链接、sandbox／捕获、真实capture的Go对账或新head CI；这些历史限制不能因离线门禁自动取消。2026-11-06后续真实执行结果见下文，发行与桌面实机验收仍未完成。

## 默认聚合检查补齐（2026-11-06）

独立复核发现前一候选 `fe31f2c` 的19项 oracle 合同只进入 macOS workflow，没有进入默认 `scripts/check`；此前工作流映射不能代替默认 fast gate。本补充不改变生产 runner、任何输入 pin 或真实症状要求。

方案：新增单独 `TestSwiftNameOracleCompilationContract`，直接执行现有 `test_oracle.py` 全部19项，失败一律 `t.Fatal`；显式验证 `NOONMARK_MYGO_BUILD_ROOT` 是已有、干净绝对路径，解析后与完整源码根互不重叠，再将其作为 Python `RUNNER_TEMP`。Python 每例仍 exclusive-create 独立临时目录，不复用或写入源码。缺 Python、缺 BUILD 或边界错误不得 skip。

将该 Go 入口登记到既有 `gates.tsv`，不放宽其五列格式、Go 函数验证或 `scripts/check` 聚合约束。其本地症状列仅代表合成编译失败控制流，状态明确不是实际 Swift 验收；真实 macOS symptom 仍由 `swift-oracle-gates.tsv` 单独绑定同案。

验证顺序：先登记映射，旧实现缺桥时 registry 必须红；加入桥后，在独立副本恢复旧生产 runner，桥必须因缺 package 参数而红；再对最终实现运行桥、BUILD 拒绝测试和 registry 全绿。默认脚本原本就强制运行根模块全部 Go tests，桥因此不能被默认聚合遗漏。

补充实际结果：

- 只有主registry登记、尚无桥的实现：`TestFailureGateRegistry` 红，`missing regression gate TestSwiftNameOracleCompilationContract`，Go exit 1。日志 `07-missing-bridge-red.log` SHA256 `9e978608ee8ebca8a551deca4f134998077545c88888b641480e09f742c4c7bc`。
- 独立红测副本保留最终桥／映射，但恢复 `c2ca275` 的旧 runner：registry 通过，桥实际执行19项 Python，`test_main_core_compiler_has_package_context_harness_stays_external` 报 `0 != 1`，桥 `t.Fatal`，Go exit 1。日志 `08-bridge-old-runner-red.log` SHA256 `f31027d2c300a4d7556eaa16ae098a59f3d31d56db168c4262efbacf66f2de97`。
- 最终候选执行 `go test -p=1 -race -buildvcs=false -run '^(TestFailureGateRegistry|TestSwiftNameOracleCompilationContract|TestSwiftNameOracleContractBuildRootGuard)$' -count=1 -v .`：3个 Go tests 及桥内19项 Python 全通过，Go exit 0、1.598秒；日志 `09-bridge-final-green.log`。使用官方 Go1.27.3、独立外置BUILD、既有离线依赖及仓库 `scripts/environment`。
- 此补充未再次运行完整 `scripts/check`；已实际运行它强制包含的新桥及registry，且原脚本的无过滤 `go test -race ./...` 调用由未改动的registry测试检查。不能将定向通过描述成默认全集已通过。

## 修复提交的真实证据（2026-11-06 UTC）

修复已进入commit `e35a5d5d628b7c0d0a2ece945fc18e944d344ee0`，tree `419e77381195c72c674dcb1fb64761d4952a40c2`。该精确head的 [Swift run42185254910／attempt1](https://github.com/ferrumlux/noonmark-go/actions/runs/42185254910) completed/success，19项合同和实际Core／包外harness编译、sandbox捕获、产物上传全部成功。原 [失败run42182456188](https://github.com/ferrumlux/noonmark-go/actions/runs/42182456188) 与其compile-core错误保留，不能与新artifact混用。

[artifact13486674928](https://github.com/ferrumlux/noonmark-go/actions/runs/42185254910/artifacts/13486674928) 原ZIP SHA256与GitHub digest一致，仅有预算内的capture／summary；来源、run／attempt和全部既有pins核对一致。实际Unicode16.0.0.0满足现有比较门禁，未改版本字符串或使用强制开关。正式Go比较器对真实capture执行1807项受支持字段对账，退出0、零差异，仍为`swiftVerified=false`。

完整环境、hash、108 cases／17 pairs／12实体字段覆盖和未验证项见 [portable evidence](../verify-swift-name-comparison/evidence-42185254910.json)；不把捕获成功称为完整Swift／实体codec／UI通过。Swift为独立原版研发对照工具，产品保持Go+MyGo，无Swift运行时依赖。

同head [MyGo run42185255089／attempt1](https://github.com/ferrumlux/noonmark-go/actions/runs/42185255089)于08:30:13 UTC确认completed/success，job127783952001全部steps成功。[artifact13488402534](https://github.com/ferrumlux/noonmark-go/actions/runs/42185255089/artifacts/13488402534)为6,219 bytes，SHA256 `582cfb32a6ac521e6d338ce5077c3235f48d814c50c20f0290f8e0965b552f9e`与官方digest一致，仅6个预期txt。精确e35源码的root race531.697秒、taskapp217.505秒、四模块及既有依赖／Unicode／native门禁、六目标编译、隔离CLI／Linux headless render均通过。

默认Go测试非verbose，成功日志没有逐项test名字；默认桥由未过滤root package成功和固定源码调用链确认，不能捏造逐test日志。六目标编译不等于六平台实机，完整MyGo allocation suite仍按既有合同排除而非通过。两表同步为已有`fixed_hosted_run_verified`，Python只改与主registry一致的一条精确状态断言；不改合法状态集合或任何source／workflow／比较验证。闭环补丁本地定向回归见下文，未推GitHub、触发CI或发行。

## 证据闭环补丁的本地定向回归（2026-11-06 UTC）

本补丁在e35基线的独立clone完成，不修改正式树或其他业务整合树。方案先记录再编辑；只修改文档、portable evidence、两张gate状态和Python对应的一条精确状态断言。原始失败run／红测记录保留，所有workflow、runner、source／vector／harness pins、比较器与生产算法字节不变。

使用既有官方Go1.27.3及独立外置BUILD，先载入恢复环境，再使用仓库`scripts/environment`建立已审查的离线依赖布局；隔离HOME／TMP。实际执行：

- `python3 -B ports/mygo/scripts/swift-name-oracle/test_oracle.py`：19项通过，0.099秒，exit0。日志`python-contract.log` SHA256：`1f184bf9877c30c0f8a0c2d4ed6603c9efcde9ef89deed2a137b0d311df1bd46`。
- 在`ports/mygo`运行 `go test -p=1 -buildvcs=false -run '^(TestFailureGateRegistry|TestSwiftNameOracleCompilationContract|TestSwiftNameOracleContractBuildRootGuard)$' -count=1 -v .`：三项Go gate及桥内19项Python通过，exit0，包测试0.272秒（不含冷编译等待）。日志`go-focused.log` SHA256：`8ec41f948291906a9d5783b02fb7af526119548f495bd4faaef5a2ca9d556750`。
- portable JSON与原Swift／MyGo artifact和正式比较报告逐项核对，检查大小、路径、hash和范围；`git diff --check`与全部12个比较器source pins通过。合法gate状态集合、原失败控制流和所有生产校验未放宽。

此补丁没有重跑完整`check`、全量race、UI或发行构建，没有触发新CI、推送或发布。真实hosted结果属于e35源码；后续整合此补丁仍应复跑受影响的registry／bridge门禁，不能把旧head CI当作新head回执。
