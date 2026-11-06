# 验证真实 Swift 分类名称 oracle

- task-id：`verify-swift-name-oracle`
- 等级：A
- 日期：2026-11-05（UTC）
- 状态：2026-11-06 真实 macOS 捕获在 Core 编译失败；package 上下文修复及离线回归已准备，新 commit 真实 CI 复验 **pending**。见 [修复任务](../fix-swift-oracle-package/README.md) 和 [故障案例](../../engineering/failure-cases/GO-SWIFT-ORACLE-001.md)。
- 本目录不会宣称分类名称互通或完整 Go 复刻已完成。

## 需求、现状与范围

目标是为 Go 移植提供真实上游 `ClassificationNameCanonicalizer.displayName`、`canonicalKey`、`algorithmVersion`、Swift `Character` 分段和 `String ==` 的运行证据，并检查合成 `TaskCategory`／`TaskLabel` 的直接构造及 Codable 往返。

上游固定为公开仓库 [quboliu/noonmark](https://github.com/quboliu/noonmark/tree/43e6eefaae3a04be89cab3e234d470111afd80ef)，commit `43e6eefaae3a04be89cab3e234d470111afd80ef`，完整 source tree `eca47c1c210b8b0d53556c955754710f5f2188c0`，Core tree `a1aa0252c8839d01dc69e47e05f2e69eceebc97d`。分类实现直接取自 `Sources/NoonmarkCore/TaskClassification.swift`，不从其他语言重写规范器。

上游 `NoonmarkCore` 有 14 个 Swift 文件，只依赖 Foundation／CryptoKit／Darwin；`Package.swift` 中此 target 没有额外 target 依赖。本任务不用上游整个 Package 构建，也不执行其脚本。现有 `domain-codec/oracle/FoundationCanonicalOracle.swift` 仅为 JSON formatter，不属于本任务的 Swift 名称证据。

## 实施方案

入口：`ports/mygo/scripts/swift-name-oracle/run.py`，不接受运行参数或外部 fixture。

1. 先验证固定 workflow、source manifest、合成 vectors 的 SHA256、形状和容量。
2. 只在 GitHub-hosted macOS 执行。固定 `macos-15` 和 `/Applications/Xcode_26.2.app/Contents/Developer`，缺失或实际版本不为 Xcode 26.2 时失败，不尝试其他 Xcode。
3. 在全新 `$RUNNER_TEMP/noonmark-swift-name-oracle` 建立隔离 HOME、TMP、cache、source、build、logs、evidence。拒绝目录复用、悬空或实际 work-root symlink、与源码重叠。macOS 系统 `/var` 的既有链接先解析为实际父路径。
4. 使用无凭证、无用户 Git config、无 hook 的 bare fetch，仅访问上述公开仓库的固定 commit。核对 commit、完整 tree、Core tree、Core 全部文件清单，以及 14 个源码文件、Package、LICENSE 的 Git blob、SHA256 和字节数。只向外置 source 导出白名单文件，Core 源码只读。
5. `swiftc -swift-version 6 -target <实际架构>-apple-macosx14.0 -package-name Noonmark` 编译完整、未改写的 Core 为 `NoonmarkCore` module／dylib，显式保留上游 `package` 声明所需上下文；再编译包外独立 `NameOracle.swift` 并 import 此模块，harness 不使用该 package flag。所有 module cache 显式外置，`xcrun --no-cache` 不建立全局 xcrun cache。不导入或运行 App、runtime、storage、sync、AI target，不运行 reset。
6. harness 仅从 stdin 接收经过 SHA 校验的合成 JSON；没有文件路径、网络、账户或环境输入。运行加 `sandbox-exec`：拒绝网络、外置根以外写入，以及用户常用资料目录和 `/Volumes` 读取。进程不继承凭证、原 HOME 或 DYLD 注入环境。未调用任何 production、iCloud、Keychain 或 AI API。
7. harness 直接调用上游 public API 和 Swift 标准库，Python 只校验结构、标量／UTF-8 自洽及输入身份，不实现 Swift 行为、不代替 oracle。

## 固定覆盖与输出接口

`vectors.json` 含 108 个名称、17 对等价比较：Unicode whitespace 与非 whitespace、组合标记、Prepend、ZWJ、variation selector、default-ignorable、德语／希腊语／土耳其语、Hangul、兼容字符、GB9c Indic、emoji、RI 奇偶，以及 29／30／31／32／60／61／4096／65536 个 U+0315 后接同数 U+0300。

`capture.json` schemaVersion=1：

- `cases[]` 含 `id`、`input`、`displayName`、`canonicalKey`、`displayNameOfDisplayName`、`canonicalKeyOfDisplayName`。
- 每个字符串完整保留 `text`、`utf8Hex`、`unicodeScalars`、`characterCount`、`characters[]`。Character 只记半开标量范围 `startScalar`／`endScalar` 及实际 `isWhitespace`，不重复其原文。
- `pairs[]` 保留 `left`／`right` case ID，分别对 input、displayName、canonicalKey 给出独立的 `swiftEqual` 与 `utf8Equal`。Swift 规范等价不能掩盖字节差别。
- `entityRoundTrips[]` 对六例 × 两种实体给出真实构造出的 name、key、版本、encodedByteCount，以及 decoder 的 accepted／rejected、typed error 或往返 equality 和字节 equality。
- 实体 ID 固定 `00000000-0000-4000-8000-000000000001`，时间固定 Unix 1700000000，颜色固定 `#336699`。上游内部自建的历史 UUID 不改写、不输出；不输出完整实体编码，所以其随机值不影响证据。
- 特别保留 `a\n\u0301b` 的单／双 display、key 与实体往返。是否非幂等、构造结果是否被 decoder 拒绝，必须读实际产物，不预填预期。
- `metadata` 记录源 manifest／SHA、harness／runner／vectors／workflow SHA、workflow commit、run ID／attempt、Swift／Xcode／macOS／SDK／架构／部署目标、runner image。`icuVersion` 与 `icuUnicodeVersion` 由同一实际进程通过 ICU public symbols 取得。
- Swift 分段的 Unicode 版本没有独立公开探针，明确记为 `not_exposed_independently`；ICU Unicode 版本和 `algorithmVersion` 都不能单独证明 Swift grapheme 版本。

### 捕获成功与互通成功严格分开

- `oracle_capture=succeeded`：固定上游在记录的平台／工具链上执行，结构与 provenance 校验通过。
- ICU Unicode 为 `16.0.0.0`：`unicode16_comparison=not_performed`，仍需用 Go 对实际字节、分段和实体行为逐例对账。
- 其他实际版本：`unicode16_comparison=unsupported`，仍保存真实捕获；不能据此宣称 Unicode 16、其他版本或所有平台互通。
- 失败会非零退出；可建立外置根后发生的失败写出有界 `summary.json`。编译／运行失败附最多 1024 字符的固定公开源／合成输入 stderr 尾段，替换运行根路径；不上传全部日志。

## 容量与安全门禁

固定输入为 848,504 bytes，输入硬上限 2 MiB，单例输入不超过 1 MiB、最多 128 cases／128 pairs。长例的五份完整文字／hex／标量数组加简短 Character 边界，离线体积预算约 7,240,157 bytes；这是容量估算，不是 Swift 运行结果。

harness 在写出前检查 16 MiB 上限，调用器接收后及追加 metadata 后再次检查；超限失败，不截断后冒充完整证据。`capture.json` 最多 16 MiB，`summary.json` 最多 16 KiB，均 exclusive-create。错误日志外置且每份最多 256 KiB，不在 artifact 白名单。

`.github/workflows/swift-name-oracle.yml`：

- 只触发 `main` push 中本 workflow 或 `ports/mygo/scripts/swift-name-oracle/**` 的变化，另支持手动运行，无外部输入。
- `contents: read`，无 secrets、self-hosted、PR target、发布或打 tag 步骤，30 分钟硬超时。
- 只上传 `${{ runner.temp }}/noonmark-swift-name-oracle/evidence/*.json`，保留 7 日，不含隐藏文件，不覆盖旧 artifact。
- checkout pin：`d23441a48e516b6c34aea4fa41551a30e30af803`。
- upload-artifact pin：`b7c566a772e6b6bfb58ed0dc250532a479d7789f`。
- 2026-11-05 已从官方 GitHub API 和精确 commit 的 [checkout action.yml](https://github.com/actions/checkout/blob/d23441a48e516b6c34aea4fa41551a30e30af803/action.yml)／[upload-artifact action.yml](https://github.com/actions/upload-artifact/blob/b7c566a772e6b6bfb58ed0dc250532a479d7789f/action.yml) 核实两者采用 Node 24；本任务不沿用 Node 20 的 upload-artifact v4。
- 不改 `mygo.yml`、`release.yml` 或任何发布门禁。

## 本地验证与真实执行边界

2026-11-05 Linux 已通过：

- `python3 -B ports/mygo/scripts/swift-name-oracle/test_oracle.py`：16 项单元测试，含来源替换／路径越界、work-root 重叠／symlink／复用、输入污染／容量、13 种不安全 workflow 变异、标量／UTF-8／Character 范围不自洽、无外部路径参数、非 macOS／self-hosted、Xcode 缺失／版本不符不降级、证据容量／exclusive-create、Unicode 版本不等于互通结论等负例。
- `actionlint 1.7.7 -shellcheck='' .github/workflows/swift-name-oracle.yml`：通过。没有 shell block；两条 run 均为固定 Python 入口。
- Python AST parsing：通过，无 in-tree pycache。
- source manifest 与只读 reference 的 commit、tree、14 个 Core 源码及 Package／LICENSE 字节和 Git objects 完整核对：通过。
- Linux 运行入口非零退出 `macos_required_not_swift_verified`，未伪造 Swift 结果。

这些测试只证明捕获入口的离线契约。本地 Linux 没有 Swift／Darwin。2026-11-06 的 [真实 run](https://github.com/ferrumlux/noonmark-go/actions/runs/42182456188) 已执行 Swift 编译，但因缺少 package 编译上下文失败；harness、macOS sandbox 与真实上游捕获尚未执行。修复整合并 push 后须跟踪该精确 commit 的新 workflow 到终态、读取 artifact 的实际平台／ICU／版本和所有向量，再安排 Go 对账。workflow 绿色也不等于互通绿色。

## 风险、回滚与后续

- Xcode 26.2 从 hosted image 消失时明确失败，须另行评估 runner／工具链契约，不能静默换版本。
- `sandbox-exec` 属于 macOS 平台工具，未在本 Linux 环境验证；不可用或策略失败时入口失败，不绕过隔离。
- Swift grapheme 与 ICU 数据版本可独立变化。固定 Go Unicode 16 只可对捕获平台逐例验证，不可外推全部 macOS／Swift。
- `algorithmVersion`、非幂等 display 与直接实体 constructor／decoder 的差异须与完整业务 workflow 接线分开审计。
- 如需撤回，只撤回本任务七个新文件；不改变现有 Go 功能或 release 工作流，不接触用户资料。此改动没有发布行为。

## 独立审核与正式整合

2026-11-05 20:58 UTC，独立审核已完成28项离线测试（含12项新增contract检查）、actionlint1.7.7、上游16文件byte/blob/tree与官方actions精确pins核验，未发现可复现阻断。真实Swift、Darwin sandbox与Go对账仍未执行。正式整合按七文件清单逐个核验SHA，仅将本说明放到ports/mygo/docs；16项原测试与actionlint在正式树重新通过。没有更改既有Go业务代码或release门禁。16MiB是最终证据输出上限，不是subprocess的实时内存配额。
