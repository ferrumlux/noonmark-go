# GO-CLASS-004：对账输出必须先通过严格类型与内部一致性校验

状态：已修复（本地工具回归；未推送、未取得真实Swift capture或发行）。修复commit：1dd93a2809eb3479eada197e1ff99e253cc66e57。

首次发现：2026-11-06（新加坡）。引入版本为未提交的离线对账候选，没有对应 Git commit，author／committer 不适用；真实自然人操作者未知。只有合成工具验证受影响，没有真实 Swift 结论或生产资料被修改。

独立检查发现 Go schemaVersion=true／1.0 被当成合法版本1，因为 Python 的宽松数值相等；unicodeScalars 的 float 没有在结构层拒绝；pair 的 utf8Equal 与其自身 Go 文本字节矛盾时只被记录为结果差异。不能将无效或内部矛盾的输出送进后续语义比较，更不能让 bool／float 冒充协议整数。

修复在两端比较前严格校验：schemaVersion 必须 type is int；所有 case／entity scalar 必须是合法 Unicode scalar 的精确整数，不接受 bool／float；文本、UTF-8 hex、scalar 逐一自洽。pair 的字节相等布尔根据该端文本重新核对，字节相同却规范相等为 false 也拒绝。只有合法、内部一致的两组结果才计算差异。

独立原回归 test_go_schema_requires_exact_integer、test_go_scalars_require_exact_integers、test_go_pair_utf8_flags_are_internally_consistent 及其他形状／未比较边界检查原样保留；另补全五类 case 文本与两类实体字段类型负例。原红日志 regression-suite.stderr 保留，不以修改预期为通过。

永久 fast／symptom gate 均为 TestSwiftNameComparisonRegressionGate，登记于 gates.tsv，经 scripts/check 通过run-tests.py在全新独立checkout中实际运行全部 test*.py。28项当前候选测试中的1807字段比较只验证 synthetic fixture 和工具行为，不代表真实 Swift 或完整分类 decode／wire 互通。Character 分段、空白属性及计数等未比较项仍明确保留。

本工具没有进入发行。回滚可移除对账工具与聚合入口，不改原始 capture、oracle、生产格式或用户资料。关闭案例须固定修复 commit 并完成正式原症状和受害路径回归；未知版本仍必须 unsupported，不能降级为通过。

正式闭环：独立复审通过，正式scripts/check退出0（root race198.074秒），其中新独立checkout完整28项26.883秒、源hash不变且无noonmark-go相邻库；六目标编译及Linux隔离入口通过。原始红证据和五项独立回归保留，最新整合证据见docs/tasks/verify-swift-name-comparison/README.md。原“待commit”描述属于早期候选状态；本地修复不代表Swift语义对账已执行。
