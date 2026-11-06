# 验证Swift名称对账工具

- task-id：verify-swift-name-comparison
- 级别：P，仅隔离研发验证工具；不开放分类catalog、生产资料或发行。
- 需求：对精确来源的Swift capture与固定Unicode16 Go算法做可审计字段比较；未知版本不能冒充通过。
- 现状：2026-11-06 真实 Swift capture 已取得；正式比较器对固定向量的1807项受支持字段全部吻合，`supported_fields_match`、`swiftVerified=false`。这不是完整Swift、实体codec、分类业务或UI对等；同head MyGo CI亦已成功；当前文档／registry补丁只作本地定向回归。详见本页最新验证记录及 [portable evidence](evidence-42185254910.json)。

## 设计与来源

公开上游基线43e6eef，复用固定108 cases／17 pairs／12实体字段输入，不改变oracle向量或工作流。工具先核验源码pin、完整来源manifest、capture／summary字节hash及调用者指定的run身份，再拒绝错误类型、形状、UTF-8、scalar、版本或内部矛盾。只从已验证源码bytes建立六文件只读编译snapshot，实际Go编译及执行后复查清单和hash。支持版本逐项比较1807项，来源、每一差异及未比较范围写入外置报告。

caller的hash与run身份不是远端真实性证明，必须另查真实GitHub成功run、官方artifact digest和实际下载bytes。工具始终保留swiftVerified=false，不从合成结果认证真实Swift或完整分类互通。Character分段／whitespace／count、实体decoder／完整wire、prepared mutation等不在语义对账范围，详见scripts/swift-name-compare/README.md。

## 风险与回滚

源／输出与缓存由已固定helper在首次写入前验证；所有合成资料、工具二进制、日志及快照外置。固定字节编译封闭正常源变动窗口，不是同UID恶意并发或被替换工具链的防护边界。源码与测试仍属于受信任项目内容。无需迁移；回滚移除独立工具和其root测试入口，不动oracle、runtime或用户资料。不进行灰度或生产监控，只用有界fixture、失败退出和逐项证据。

## 永久验证

TestSwiftNameComparisonRegressionGate由scripts/check强制调用run-tests.py；每次在全新最小checkout执行全部28项，并证明相邻不存在真实noonmark-go仓库。测试输出在另一外置子树，结束核对源码不变及无新增相邻库。原独立5项测试保持字节不变，旧source-binding／类型／可移植性症状红证据均保留。错误路径在mkdir前拒绝，未标识的同名外置目录正例允许。

已发生的缺陷分别登记GO-CLASS-003、004、005，各有原始红、实际修复绿及永久聚合门禁。候选与独立复审并不代替正式副本整合门禁；状态及修复commit由整合后回填。该工具初始整合时（2026-11-05）没有真实Swift执行、新GitHub CI或非Linux实机验收；当时待批远端写入没有重试或绕行。2026-11-06 的真实验证只更新下文列出的有限范围。

## 正式整合验证（2026-11-05 UTC）

独立复审先真实发现旧路径fixture依赖相邻仓库并判红，修复后在陌生checkout全28项通过27.376秒，独立Go race wrapper通过28.039秒。另证源内输出拒绝且未建目录、坏Go工具导致真实子测试失败并原样exit5，没有假成功。八份候选文件hash与最后冻结一致，原五项独立回归未改。

正式副本99839d9基线选择性整合八份脚本文件、最终Go wrapper、三例故障登记与本任务文档。scripts/check真实退出0，主模块race198.074秒，四模块race／vet、Unicode16完整离线门禁、51项dependency及native UI/popup全部通过。内嵌新独立checkout实际全28项26.883秒，standalone-result确认exit0、sourceUnchanged和noNoonmarkGoSiblingAfter为true、swiftVerified为false；stdout／stderr及来源hash清单保留在外置整合证据中。

六目标cross-build及Linux隔离create/reopen/day+pool渲染／拒绝覆盖入口退出0。发行工具17单测、17工作流安全负例、shell语法和三个工作流actionlint通过；开发CI的8个合成runner fixture、9个workflow负例及有界输出失败fixture通过。没有实际触发新GitHub CI、Mac oracle或发行；GitHub待批写入未重试。

外置证据前缀noonmark-swift-compare-merge-20261105，包含integrated-check/cross/cli.log、integrated-standalone-result.json、integrated-standalone-source-manifest.json、integrated-tests.stderr以及release-unit/release-contract/actionlint/development-ci-contract.log。此前scripts目录的作者记录描述当时未整合状态；本节保留2026-11-05整合历史，后续真实capture与CI结果以下节为准。

## 真实 capture 与有限 Go 对账（2026-11-06 UTC）

固定源码修复 commit 为 `e35a5d5d628b7c0d0a2ece945fc18e944d344ee0`。已独立核验 [Swift run 42185254910／attempt 1](https://github.com/ferrumlux/noonmark-go/actions/runs/42185254910) 与 [artifact 13486674928](https://github.com/ferrumlux/noonmark-go/actions/runs/42185254910/artifacts/13486674928)：run及所有steps成功，ZIP 20,799 bytes与官方digest一致，只含预算内的capture.json和summary.json。来源manifest、workflowCommit、runID／attempt及workflow／runner／vectors／harness pins全部吻合。

真实平台为 macOS15.8.2 arm64、Xcode26.2 build17C52、Swift6.2.3、SDK26.2、runner image20261028.0489.1；ICU76.1.0.0、ICU Unicode16.0.0.0。summary为`oracle_capture=succeeded`、`stage=complete`；capture本身保留`unicode16_comparison=not_performed`，不改写原证据。

随后以未改动的正式`compare.py`、官方Go1.27.3 linux/amd64及独立外置BUILD执行真实capture对账，退出0。108 cases与12条实体字段的564条TextRecord共1692项、17 pairs的102项、13项算法版本，总计1807项逐项`equal=true`，零差异；`swiftVerified=false`保持不变。完整报告SHA256为`630b00d3eee0b6e931724856d3a6d2ba234c3068969dbf5a2bc0c73febebb5d1`。

真实Swift实体捕获有6条accepted与6条dataCorrupted rejected，后者是empty、only-di、display-non-idempotent各两种实体。这是原版实际行为，未改成全往返成功；Go比较器没有调用实体decoder／encoder，这些capture状态只作结构验证；完整wire未捕获，也未作Go对账，均不属于1807项语义对账。

仍未比较Character边界／count／逐Character whitespace、完整实体codec、分类prepared workflow／history／ownership／catalog、迁移、其他输入／版本和macOS／Windows产品运行／UI。`swiftGraphemeUnicodeVersion=not_exposed_independently`，不能从ICU16推断Swift分段版本。Swift只作为独立研发对照工具，不加入Go+MyGo产品运行时。

[portable evidence](evidence-42185254910.json)只保存公开run／artifact身份、hash、环境、覆盖与边界，不含绝对路径、临时下载链接或原始7MB capture。原始artifact保留期7日；此小型索引不替代原capture／完整报告，也不能单靠hash证明真实性。

同head [MyGo run42185255089／attempt1](https://github.com/ferrumlux/noonmark-go/actions/runs/42185255089)已completed/success，job全部steps成功；精确源码身份、6个白名单txt及6,219-byte artifact官方digest均核对一致。root race531.697秒、taskapp217.505秒、四模块和既有依赖／Unicode／native门禁、六目标编译、隔离CLI与headless render均通过。完整MyGo allocation suite仍属既有排除项，六编译与headless结果不能外推桌面实机。该CI结果仅属于e35，当前文档／registry补丁的本地定向回归见[修复任务](../fix-swift-oracle-package/README.md)。
