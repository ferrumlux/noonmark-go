# 验证Swift名称对账工具

- task-id：verify-swift-name-comparison
- 级别：P，仅隔离研发验证工具；不开放分类catalog、生产资料或发行。
- 需求：对精确来源的Swift capture与固定Unicode16 Go算法做可审计字段比较；未知版本不能冒充通过。
- 现状：已有独立canonical16和Mac oracle工作流源码，但真实Swift capture尚未取得；本工具的全部已执行验证均为明确标记的合成fixture。

## 设计与来源

公开上游基线43e6eef，复用固定108 cases／17 pairs／12实体字段输入，不改变oracle向量或工作流。工具先核验源码pin、完整来源manifest、capture／summary字节hash及调用者指定的run身份，再拒绝错误类型、形状、UTF-8、scalar、版本或内部矛盾。只从已验证源码bytes建立六文件只读编译snapshot，实际Go编译及执行后复查清单和hash。支持版本逐项比较1807项，来源、每一差异及未比较范围写入外置报告。

caller的hash与run身份不是远端真实性证明，必须另查真实GitHub成功run、官方artifact digest和实际下载bytes。工具始终保留swiftVerified=false，不从合成结果认证真实Swift或完整分类互通。Character分段／whitespace／count、实体decoder／完整wire、prepared mutation等不在语义对账范围，详见scripts/swift-name-compare/README.md。

## 风险与回滚

源／输出与缓存由已固定helper在首次写入前验证；所有合成资料、工具二进制、日志及快照外置。固定字节编译封闭正常源变动窗口，不是同UID恶意并发或被替换工具链的防护边界。源码与测试仍属于受信任项目内容。无需迁移；回滚移除独立工具和其root测试入口，不动oracle、runtime或用户资料。不进行灰度或生产监控，只用有界fixture、失败退出和逐项证据。

## 永久验证

TestSwiftNameComparisonRegressionGate由scripts/check强制调用run-tests.py；每次在全新最小checkout执行全部28项，并证明相邻不存在真实noonmark-go仓库。测试输出在另一外置子树，结束核对源码不变及无新增相邻库。原独立5项测试保持字节不变，旧source-binding／类型／可移植性症状红证据均保留。错误路径在mkdir前拒绝，未标识的同名外置目录正例允许。

已发生的缺陷分别登记GO-CLASS-003、004、005，各有原始红、实际修复绿及永久聚合门禁。候选与独立复审并不代替正式副本整合门禁；状态及修复commit由整合后回填。当前没有真实Swift执行、新GitHub CI或非Linux实机验收，待批远端写入不能重试或绕行。

## 正式整合验证（2026-11-05 UTC）

独立复审先真实发现旧路径fixture依赖相邻仓库并判红，修复后在陌生checkout全28项通过27.376秒，独立Go race wrapper通过28.039秒。另证源内输出拒绝且未建目录、坏Go工具导致真实子测试失败并原样exit5，没有假成功。八份候选文件hash与最后冻结一致，原五项独立回归未改。

正式副本99839d9基线选择性整合八份脚本文件、最终Go wrapper、三例故障登记与本任务文档。scripts/check真实退出0，主模块race198.074秒，四模块race／vet、Unicode16完整离线门禁、51项dependency及native UI/popup全部通过。内嵌新独立checkout实际全28项26.883秒，standalone-result确认exit0、sourceUnchanged和noNoonmarkGoSiblingAfter为true、swiftVerified为false；stdout／stderr及来源hash清单保留在外置整合证据中。

六目标cross-build及Linux隔离create/reopen/day+pool渲染／拒绝覆盖入口退出0。发行工具17单测、17工作流安全负例、shell语法和三个工作流actionlint通过；开发CI的8个合成runner fixture、9个workflow负例及有界输出失败fixture通过。没有实际触发新GitHub CI、Mac oracle或发行；GitHub待批写入未重试。

外置证据前缀noonmark-swift-compare-merge-20261105，包含integrated-check/cross/cli.log、integrated-standalone-result.json、integrated-standalone-source-manifest.json、integrated-tests.stderr以及release-unit/release-contract/actionlint/development-ci-contract.log。此前scripts目录的作者记录描述当时未整合状态；最新正式集成状态以本节和三份故障案例为准。
