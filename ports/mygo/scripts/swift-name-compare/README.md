# Swift 名称 capture 离线对账

这是已整合的离线工具。**2026-11-06 已取得真实 Swift capture，固定108 cases／17 pairs／12条实体字段共1807项对账吻合；不是完整 Swift／实体 codec／UI 对等。** 当前真实结果与边界见 [验证任务](../../docs/tasks/verify-swift-name-comparison/README.md) 和 [portable evidence](../../docs/tasks/verify-swift-name-comparison/evidence-42185254910.json)。 单测从 Go 输出构造明显标为 `synthetic-test-fixture-not-swift` 的输入，只证明验证器、差异检测和重放流程；生产 CLI 必定拒绝该类输入，不提供接受合成结果的开关。

不改 oracle workflow、`run.py`、Swift harness 或固定向量，不接入 engine、runtime、codec、非空 catalog 或生产资料。只依赖 Python 3.10+ 标准库、固定 Go 1.27.3，以及本仓库已审查的 `canonical16` 纯 Go 实现，没有新增第三方依赖。

## 新增文件与合并方式

只需将本目录八个文件合并到正式仓库同一路径：

- `compare.py`：严格验证、外置构建、对账、证据与重放入口。
- `probe.go`：只调用 canonical16 公开 API 的离线 Go probe。
- `pins.json`：现有 oracle 五份文件和 canonical16 四份生产源码、其路径门禁、module 定义及本目录probe的固定 SHA-256。额外未固定的 Go／汇编等编译输入也会被拒绝。
- `test_compare.py`：完整原向量的合成fixture、篡改负例和版本／路径门禁。
- `test_review_regressions.py`：独立审查发现的schema／scalar／pair内部一致性永久负例。
- `run-tests.py`：先验证边界，再把完整测试搬到全新外置独立源码布局执行，保留逐文件hash、日志及原样退出码。
- `FAILURE_CASES.md`：源绑定及类型门禁故障案例、根本修复和必须追加的验证入口。
- `README.md`：范围、入口与集成说明。

`pins.json` 自身的 SHA-256 也固定在 `compare.py`。任何受保护源文件变化均拒绝运行，不能修改 hash 让新版本自动通过；应另行审查新版 oracle／算法契约。复制的 domain-extensions、domain-core 和 oracle 文件只是隔离测试依赖，不是本变更的修改文件。现有 canonical16 独立完整验证入口仍是它自己的合并门禁，本工具不取代它。

## 真实证据到达后

先从可信的实际 oracle run 获取原 `capture.json` 和 `summary.json`，由主任务回读对应成功 run、官方 artifact digest 和下载的真实 bytes，独立确认 workflow commit、run ID、attempt、artifact 来源及 capture SHA-256。CLI 要求这些值，不能从待检 capture 自动接受身份。哈希只能绑定字节，不能签名或认证 GitHub run；本工具不访问 GitHub，也不会触发远端操作。

```sh
python3 -B ports/mygo/scripts/swift-name-compare/compare.py \
  --capture /external/original-artifact/capture.json \
  --summary /external/original-artifact/summary.json \
  --capture-sha256 ACTUAL_64_HEX_CAPTURE_SHA256 \
  --workflow-commit ACTUAL_40_HEX_WORKFLOW_COMMIT \
  --run-id ACTUAL_RUN_ID --run-attempt ACTUAL_ATTEMPT \
  --go /absolute/path/to/go1.27.3/bin/go \
  --build-root /external/noonmark-swift-name-comparison
```

上面占位符必须换成实际已核实值。没有真实 artifact 时不要运行假 capture 来生成成功报告。源与输入先只读验证；失败时退出2且不启动Go，不创建构建目录。源、summary、输入 identity、shape 或metadata不全都拒绝。证据完整但 ICU Unicode 版本不等于16.0.0.0时，stdout输出 `unsupported`、0项检查，退出3且不启动Go；不能修改版本字符串或强制启用比较。

版本16只表示可以比较固定向量，不证明Swift Character实际采用Unicode16。ICU库版本单独记录，不将库主版本或 `algorithmVersion` 当作Swift分段版本。捕获的 `swiftGraphemeUnicodeVersion` 必须保持 `not_exposed_independently`。

通过输入门禁后，工具在外置root下创建互不重叠的 `runs/`、`gopath/`、`gocache/`，每次在 `runs/` 下独占新建0700的 `run-*`。拒绝源树／相邻源码库、源码祖先、非规范路径、symlink、hardlink和special file；不复用旧run。Go使用外置临时目录、禁止模块网络和自动工具链下载、CGO=0、GOWORK=off、GOENV=off、只读module模式，并关闭VCS stamping。日志、binary、原capture/summary、固定向量、probe原始输出与 `comparison.json` 全部留在新run中。

构建不会再打开可变候选源码。`load_inputs` 保留所有已验证bytes，`freeze_build_sources` 只用这些bytes建立外置 `source-snapshot/`：固定的4份canonical16源码、固定probe、内置最小离线go.mod（保持相同module import路径，不引入父module未使用的依赖）。文件0400、目录0500，全部6份实际应用／probe／module编译输入的hash、size和来源写入 `source-snapshot-manifest.json` 并纳入报告。Go cwd及相对入口只指向snapshot，编译前后和probe执行后验证snapshot的完整清单与字节。固定路径helper也从已验证bytes加载，不在校验后重新读取源文件执行。

这个机制防止正常编辑或新增源码造成证据绑定错误，**不是针对同UID恶意并发进程的安全沙箱**。Go标准库和编译器属于另行记录的固定工具链，并非这6份源码snapshot的一部分；报告保留Go版本、Go可执行档SHA-256和构建flags。

退出0仅表示 **supported_fields_match**，退出1表示 **supported_fields_mismatch**。两者都不是完整Swift／实体对等。报告有逐字段状态、两边值的确定性JSON SHA-256、精确覆盖、未比较边界、源码／capture／summary／probe源码／binary／输出 hashes和可重放命令。`swiftVerified` 始终为false，防止被自动化误当作整体Swift门禁通过。失败输出在stderr，未知Unicode输出在stdout。构建后失败可能留下外置run日志，不会产生成功报告。

## 精确覆盖

原始固定输入是108 cases、17 pairs，源byte hash不变。每case直接从固定原始字符串重新计算：

1. input原文。
2. `DisplayName(input)`。
3. `CanonicalKey(input)`。
4. `DisplayName(DisplayName(input))`。
5. `CanonicalKey(DisplayName(input))`。

两次display不能省略；`display-non-idempotent` 的第二次display会改变组合mark。12条直接TaskCategory／TaskLabel实体字段是6个既定case × 2个类型，计算 `name=DisplayName(raw)`、`key=CanonicalKey(name)` 及算法版本。**不调用**会拒绝空display/key的 `Canonicalizer.Canonicalize`，因此正确覆盖原始initializer允许的empty与only-di。

- 540条case TextRecord + 24条实体 TextRecord = 564条。每条分别比对text、utf8Hex、unicodeScalars，共1692项。
- 17 pairs × input/display/key × canonical equivalence/byte equality = 102项。Swift `==` 对应Go `Equivalent`，字节等同对应Go字符串字节相等，不把两者混为一谈。
- 12个实体算法版本 + 1个公开算法版本。
- 总计1807项；报告列出每一项而非预填全局成功。

严格检查exact keys、JSON重复member、UTF-8／孤立surrogate、NaN／Infinity、整数与boolean区分、缺项／重复／额外／顺序、原始输入、scalar／utf8一致、Character记录连续完整覆盖、pair字节自洽、实体按decode分支shape、字段大小及16MiB capture上限。摘要必须绑定原capture byte hash和准确计数。capture/source/harness/runner/workflow metadata、固定实体输入、Xcode26.2、macOS15、SDK与架构／target、image版本、run身份均须齐全。

## 明确未比较

- Go公开API没有暴露Character边界／count／逐Character `isWhitespace`。只校验capture中的连续覆盖、count／type自洽，不证明边界、whitespace值或Swift分段版本。合成fixture故意用非真实分段，测试确保它们不能被报告成已对账。
- 实体完整编码byte、decoder、decodeError、round-trip标志和实体等价没有对应Go调用。本工具只验证这些capture字段的结构；其值不会计入1807项。
- 不证明prepared create/resolve/rename、历史、名字所有权、catalog、迁移、runtime、macOS/Windows实机Go行为或其他输入／版本。没有安装任何生产算法。
- 不认证远端artifact真实性。源manifest必须逐字段对应固定清单，但仅匹配元数据无法证明实际编译运行；必须独立核实原artifact来源。

## 离线测试

```sh
GO=/absolute/path/to/go1.27.3/bin/go \
NOONMARK_SWIFT_COMPARE_TEST_ROOT=/external/noonmark-swift-name-comparison \
python3 -B ports/mygo/scripts/swift-name-compare/run-tests.py
```

测试使用原始全部108/17输入，不删长向量，也不改变oracle。fixture只在外置run中生成，文件名包含 `SYNTHETIC-NOT-SWIFT`，metadata及报告标为synthetic。测试用局部mock覆盖captureKind入口以检查完整写报告流程；生产CLI没有该参数。新增回归仅在外置合成fixture中改变module/probe/canonical并放置benign stderr sentinel，证明已验证snapshot的真实Go编译不受其影响；不会对正式或候选源码注入。相同fixture重放两次验证独立run、原证据不覆盖、字段和provenance一致；真实未知Unicode证明0项Go比较；3项篡改输出差异必须退出1。另测源hash、校验后源码变动与只读snapshot、Go schema/scalar严格类型、Go pair内部字节一致性、metadata、计数、JSON重复key、Unicode、缺漏／重复、pair／实体、版本、summary和source／symlink／hardlink／FIFO路径负例。

CI集成时先加这条本地离线测试命令，正式真实capture比较作为独立、明确输入身份的后续步骤。**不要改变现有oracle workflow，也不要将合成fixture成功替代真实capture比较。**

正式聚合门禁必须调用 `run-tests.py`，不能只在当前checkout做discovery。该入口先用已验证路径helper拒绝不安全输出，再从固定依赖bytes及当前全部 `test*.py` 创建全新最小checkout；其父目录起初只有checkout，没有 `noonmark-go` 相邻库。测试输出位于另一外置子树，结束核对源码hash及无新建相邻库，保留 `standalone-source-manifest.json`、`standalone-result.json`、stdout/stderr。单独debug仍可直接discover，但不能代替这个布局回归门禁。

路径负例全部在外置合成布局patch ROOT／MODULE，以 `.git/HEAD` 或 `ports/mygo/AGENTS.md` 明确标识不同名字的源码库；拒绝必须发生在mkdir／mkdtemp之前，逐项核对fixture清单和bytes不变。正例确认没有源码标识的 `noonmark-go` 名字只是普通外置目录，且两次run独立。helper绑定永久测试禁止验证后重新调用read_file。
