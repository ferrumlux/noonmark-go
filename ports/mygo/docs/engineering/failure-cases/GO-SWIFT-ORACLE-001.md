# GO-SWIFT-ORACLE-001：独立 swiftc 缺少 package 编译上下文

- 状态：**已修复（独立oracle编译／捕获范围）**；修复commit `e35a5d5d628b7c0d0a2ece945fc18e944d344ee0` 的真实macOS原始症状与同head MyGo CI均成功，另有1807项有限Go字段对账吻合。不是完整Swift／实体codec／跨平台UI或发行验收；本证据补丁只作本地定向回归。
- 首次真实发现：2026-11-06 07:41 UTC，GitHub hosted [run 42182456188](https://github.com/ferrumlux/noonmark-go/actions/runs/42182456188)／[job 127774977256](https://github.com/ferrumlux/noonmark-go/actions/runs/42182456188/job/127774977256)。
- 引入提交：`73aaaa1d6cb28096ffd012cf50bb139b2213b454`，`ci: capture exact upstream Swift classification behavior`。Git author／committer 均为 `Codex <codex@openai.com>`；Git 身份不证明实际操作者，实际修改者无法仅由当前证据确认，记为未知。
- 该入口从引入提交到复验基线 `c2ca275d8e43975b82f71a633decfe6d0dc88b97` 字节未变。修复commit为`e35a5d5d628b7c0d0a2ece945fc18e944d344ee0`；新hosted症状证据见下文，不能引用旧run作为修复成功证据。

## 用户症状与影响

原始症状是Swift oracle工作流红灯，无法产出真实Swift capture，因此当时不能宣称Swift名称行为已完成对账。应用、用户资料和 Go 运行路径不受此编译入口修正影响。本案未运行 App、production、iCloud、Keychain 或 AI。

## 原始证据与排除假设

真实 runner 为 macOS 15.8.2、`macos-15-arm64` image `20261104.0514.1`；固定 Xcode 26.2 检查和上游来源校验已通过，16 个离线合同也通过。`compile-core` 随后失败，stderr 明确指出 `TaskClassification.swift` 的 `copyTaskAsIndependentPoolTask` 和 `inheritCurrentClassification` 的 `package` 访问级别需要 `-package-name`。完整公开 CI log 的 SHA256 为 `6369acc84e4906e9deb1ccdd43eef763dc902fba7eaae176bbacb7879076702f`。

因此排除“缺少 Xcode”“尚未启动 runner”“权限或计费使编译未执行”作为此红灯解释。修复只针对已证实的首个编译错误，不预判后续编译、链接或 sandbox 一定通过。

## 破坏机制与根因修复

`run.py` 手动调用 `xcrun swiftc`，没有经过 SwiftPM 的编译参数生成。`-module-name NoonmarkCore` 仅提供模块身份，不能提供 `package` 声明所需的包上下文。旧测试覆盖工具链和资料边界的拒绝路径，没有走到实际编译命令。

固定上游 [Package.swift](https://github.com/quboliu/noonmark/blob/43e6eefaae3a04be89cab3e234d470111afd80ef/Package.swift) 的 `name` 是 `Noonmark`，Core 是无额外依赖的 target。只读获取后核对 6689 bytes、SHA256 `567ba7c660ac4ea55f09ea14052dfb164a21460f56f51adb370e0c429cc312c7`、Git blob `3ac73de79c116ed9d498137cdb90e44399b5334f`，均与既有 source manifest 相符。

修复仅在原 `compile-core` 命令增加 `-package-name Noonmark`，显式建立该隔离模块的包上下文。此名称不是对 SwiftPM 原始命令行 identity 的测量结论。独立 harness 不加入同包身份、`@testable` 或 SPI，继续从包外调用 public API。上游 `package` 源码和所有 manifest／tree／blob／hash 均不改，未引入任何 fallback。

比较器的 runner pin 随入口精确更新，再更新 `compare.py` 的 `PINS_SHA256`。这是保持来源绑定所需的两层更新，没有放宽旧 capture 的接受条件，也不修改其他固定输入。

## 自动化门禁与真实复验

默认 fast gate 登记于同目录 `gates.tsv`：`TestSwiftNameOracleCompilationContract` 在根 Go tests 中直接执行19项 Python oracle 合同；`scripts/check` 强制执行根模块全部 tests，所以不能遗漏本案。`TestFailureGateRegistry` 的原有五列规则和 Go 函数检查不变。该表的本地症状列只登记同一合成失败控制流入口；案例fixed状态另有真实hosted证据支持，不能把该本地gate本身称为Swift执行验收。真实 macOS symptom 另在 `swift-oracle-gates.tsv` 绑定同案和同一 Go fast gate；Python 映射测试校验两表连接、case 文件、固定 workflow SHA、合同 step 先于实际捕获 step。

- Fast gate：`test_main_core_compiler_has_package_context_harness_stays_external` 直接调用实际 `main()`，拦截它生成的 subprocess argv，断言 core 只有一个固定包名、完整14个源文件、Swift 6／macOS14／外置 cache，harness 无 package flag。旧入口先跑得到 `AssertionError: 0 != 1`，修复后通过。
- 失败控制流 gate：`test_main_core_compile_failure_stops_before_harness_and_capture` 让 core 返回17，验证没有 harness/capture；前一测试让 harness 返回17，验证没有 capture。两者只生成 failed summary，`stage=compile`、`unicode16_comparison=not_performed`。未知 subprocess 调用直接使测试失败。
- Fast gate 只使用显眼合成 Git／源码和模拟编译响应，不生成 `capture.json`，不证明任何 Swift 编译或运行成功。
- 默认聚合入口：`scripts/check` → 根模块 `go test -race ./...` → `TestSwiftNameOracleCompilationContract` → `test_oracle.py` 全19项。BUILD 必须是既有、绝对、干净且解析后与整个源码仓库互不重叠的外置目录；缺 Python 或其他执行错误一律 `t.Fatal`。`TestSwiftNameOracleContractBuildRootGuard` 覆盖空值、相对路径、不存在、普通文件、源码内／相等／祖先、带尾分隔符、指向源码的 symlink 与正常外置目录。
- 平台聚合入口：`.github/workflows/swift-name-oracle.yml` 的 capture job，先执行 `python3 -B ports/mygo/scripts/swift-name-oracle/test_oracle.py`，再执行真实 `python3 -B ports/mygo/scripts/swift-name-oracle/run.py`。
- 原始症状 gate：上述真实 runner 必须在新修复 commit 的 GitHub-hosted macOS／Xcode 26.2 上完成 Core／harness 编译、sandbox 执行和固定产物校验；不许 skip、不换编译器、不用 Python fixture 代替。

补充发现：前一候选 `fe31f2c` 只有 workflow 合同入口，没有默认 Go→Python 桥，不能视为满足默认 fast gate。父级审核指出后补齐独立桥及主 registry，保留旧提交和红测证据，不改写先前覆盖范围。

操作和证据要求见 `../../tasks/fix-swift-oracle-package/README.md`。真实 capture 成功仍不等于 Go 对账成功，Unicode 版本不符仍须标为 unsupported。

## 发行、回滚与教训

真实macOS症状与同head MyGo CI均已转绿且修复commit已记录，本案仅在独立oracle编译／捕获缺陷范围闭环。发行验收不能消费 failed summary 或合成 capture。本修复不修改既有 release 门禁，不触发发布，也不扩大 Git 权限。

回滚仅撤回此独立修复及配套两层 pin，保留失败证据，无数据迁移。以后手动编译包内源码必须保留其访问级别所需编译上下文，并测试真实调用入口发出的 argv；命令合同通过后仍要执行目标平台真实编译。

## 真实症状复验与闭环进度（2026-11-06 UTC）

修复commit `e35a5d5d628b7c0d0a2ece945fc18e944d344ee0` 的 [run42185254910／attempt1](https://github.com/ferrumlux/noonmark-go/actions/runs/42185254910) 与全部steps成功：19项合同后，实际Core、包外harness编译、sandbox执行及有界artifact均通过，原始`compile-core_failed`症状已消除。

[artifact13486674928](https://github.com/ferrumlux/noonmark-go/actions/runs/42185254910/artifacts/13486674928) 的20,799-byte ZIP已与官方digest核对，SHA256为`cb056e61b88b9977dcf1f651232dae4c141045bf74afe56a23d0b6cebce7eda1`。仅capture.json、summary.json，未超预算或含路径穿越；summary为succeeded／complete且绑定原capture hash。实际metadata、固定source／runner／workflow／harness／vectors及run身份均一致，没有放宽pin或改原始证据。

实际macOS15.8.2 arm64／Xcode26.2／Swift6.2.3／ICU76.1.0.0／Unicode16.0.0.0；独立正式Go1.27.3 linux/amd64比较器对108 cases、17 pairs与12条实体字段记录共1807项全部吻合。`supported_fields_match`不等于完整Swift／codec／UI通过；`swiftVerified=false`保留。完整hash、来源与范围见 [portable evidence](../../tasks/verify-swift-name-comparison/evidence-42185254910.json)。

同head [MyGo run42185255089／attempt1](https://github.com/ferrumlux/noonmark-go/actions/runs/42185255089)已completed/success，job127783952001所有steps成功。6,219-byte artifact13488402534与官方digest一致，仅6个预期txt；源码身份为e35，root race531.697秒、taskapp217.505秒、四模块和既有依赖／Unicode／native门禁、六目标编译及隔离CLI／headless render均成功。两张gate映射同步为现有`fixed_hosted_run_verified`，仅同步Python主registry的对应精确断言，没有扩大合法状态集合或更改任何生产验证。

MyGo日志采用非verbose Go测试，不逐项列成功test名字；默认桥覆盖由该精确head的未过滤root测试成功和固定源码调用链共同支持。新文档／registry补丁的定向回归另记于修复任务；不能把e35的CI结果说成本补丁已获hosted验收。
