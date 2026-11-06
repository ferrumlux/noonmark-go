# 离线对账工具故障案例与追加门禁

日期：2026-11-05。全部案例、测试及证据均使用合成输入；无真实Swift执行或互通结论。正式树、oracle workflow、runtime、engine及GitHub均未修改。

## 1. 校验与编译之间的源码绑定缺口（阻断）

原实现先核验pin和canonical16编译输入清单，随后仍在可变源module目录执行Go build。独立审查在其自己的副本中，于验证后新增只向stderr打印sentinel的Go源。Go真实编译执行了该文件，但最终报告仍是1807项 `synthetic_fixture_match`，provenance未记载新增编译输入。原18项测试全绿未覆盖这个时间窗口。

原红证据由独立审查保留，文件名 `source-binding-counterexample.json`，其核心事实是：exit0、mismatchCount=0、actualGoInitSentinelInStderr=true、reportedPinnedSourcesContainExtraInput=false。修复没有重复对正式或候选源码注入。

根本修复：

- 源验证时保留原始bytes，probe本身也进入固定pin清单。
- 仅从这些bytes建立外置独占、只读编译snapshot。实际应用／probe／module输入只有4份canonical16源码、probe和内置最小离线go.mod。
- Go cwd与相对编译入口只指向snapshot，不能重新打开mutable源码；关闭自动VCS stamping，避免旁边Git目录影响编译。
- snapshot完整清单、size、hash、来源、manifest hash和工具链identity进入provenance。构建前后及probe执行后重验snapshot。
- 路径helper从已验证bytes加载，避免相同的校验后重新读取问题。

永久回归 `test_build_uses_verified_snapshot_after_source_changes` 在外置独立合成fixture改变module／probe／canonical，并加入benign sentinel；真实Go编译必须仍得到固定输出，stderr无sentinel，snapshot实际只有6个固定输入。`test_snapshot_bytes_and_inventory_rejected_if_changed` 校验外置snapshot被改后必须拒绝。这个设计防止普通源编辑导致证据错绑，不宣称抵御同UID恶意并发进程。

## 2. Python宽松数值相等导致Go输出schema遗漏（阻断）

原实现接受Go `schemaVersion=true` 或 `1.0`，因为它们与Python整数1相等。数值相等的float scalar数组也被当成有效结构，只在后续字节hash比较计为mismatch。Go pair的utf8Equal若与Go文本字节矛盾，原来也只被计为结果差异，而非先拒绝内部不自洽证据。

修复明确要求schemaVersion `type is int`、所有case／entity unicodeScalars均为精确integer list，拒绝boolean与float。Go text／UTF8／scalar先内部自洽校验，再比较；Go pair字节布尔根据Go文本重新计算核对。捕获与Go两边均拒绝“字节相同但规范等价false”的矛盾记录。

独立审查提供的5个方法原样合入 `test_review_regressions.py`；其schema bool/float、scalar float和pair字节矛盾负例必须全部转绿。另有覆盖5种case文本字段和2种实体字段的类型负例。

## 正式集成必须追加的gate

在项目自己的既有tests/vet和canonical16完整门禁之外追加：

```sh
GO=/absolute/path/to/go1.27.3/bin/go \
NOONMARK_SWIFT_COMPARE_TEST_ROOT=/external/comparison-test-root \
python3 -B ports/mygo/scripts/swift-name-compare/run-tests.py
```

必须同时执行主测试与独立复审测试，不可只挑原18个方法。构建、缓存和证据保持外置，fixture及结果醒目标synthetic。真实Swift比较仍是后续独立gate：主任务验证成功run、官方artifact digest及真实下载bytes，再将完整身份和capture/summary传入生产CLI。任何合成测试通过都不替代该gate。

## 3. 单测错误依赖正式仓库的相邻目录（GO-CLASS-005，处理中）

2026-11-06（新加坡）独立审查确认：未提交候选的 `test_bad_paths_no_write` 把 ROOT.parent/noonmark-go/outside-mygo 当作必然受保护源码。旧test_compare.py SHA-256为 `7e121e51acc5557c14bf5bf9e43b81025169619d58cc9178ccf94f73bdb81439`。陌生独立checkout没有该库，生产守卫正确允许普通外置目录，旧26测试变成25过、1失败；此候选从未进入正式远端，不存在引入commit，Git author／committer不适用，实际自然人操作者未知。原独立复审红日志与新增red.stderr保留，不能修改生产守卫使错误预期通过。

根本修复将负例迁入外置合成布局，patch ROOT／MODULE，明确创建Git及port标识，以实际路径拒绝、零mkdir／mkdtemp调用和全fixture清单／bytes不变断言。正例证明没有标识的同名相邻目录合法且run独立。并吸收helper-binding永久负例，确保prepare_run只执行先前验证的helper bytes。原五个独立回归案例保持逐字节不变。

聚合入口必须调用新增 `run-tests.py`：每次建立无相邻noonmark-go的全新独立最小checkout，执行全部test*.py并保留hash清单和真实stdout／stderr／exit。旧测试在此自动入口仍应判红；修复后28项转绿。不得仅保存本次人工搬目录证据。此门禁映射为正式 TestSwiftNameComparisonRegressionGate 的fast／symptom，GO-CLASS-005单独登记。未固定修复commit和正式聚合回归前保持处理中。

## 后续正式本地闭环

以上为候选复审时记录。正式修复commit为1dd93a2809eb3479eada197e1ff99e253cc66e57，三例已在docs/engineering/failure-cases中登记并闭环；正式全量门禁和新独立布局28项通过。最新证据见docs/tasks/verify-swift-name-comparison/README.md。原五项独立测试及全部生产比较器代码未改断言或放宽校验。该状态只指工具可靠性修复，没有真实Swiftcapture、新GitHub CI或发行。
