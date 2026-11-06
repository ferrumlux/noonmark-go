# GO-CLASS-003：对账编译必须绑定已验证的源码字节

状态：已修复（本地工具回归；未推送、未取得真实Swift capture或发行）。修复commit：1dd93a2809eb3479eada197e1ff99e253cc66e57。

首次发现：2026-11-06（新加坡）。引入版本为未提交的离线对账候选，没有引入 Git commit，author／committer 不适用。可核验记录为候选源码与独立检查证据，实际自然人操作者未知。正式运行程序与生产资料未受影响。

原实现先验证 pin 与源码文件清单，随后却在可变源码目录运行 Go build。独立合成副本在验证后加入只输出 stderr sentinel 的额外 Go 文件，实际编译执行该文件，仍得到1807项 synthetic_fixture_match，报告没有包含额外编译输入。原18项测试没有覆盖验证与使用之间的窗口；只看固定 hash 或最终结果相等不足以证明实际编译来源。

根本修复保存已经核验的 bytes，包括 probe。只从这些 bytes 创建外置独占、只读的六文件编译 snapshot：最小离线 go.mod、四份 canonical16 源码与 probe。Go 的 cwd 和入口仅指向该 snapshot，构建前后和 probe 执行后核验完整清单、大小与 hash。路径 helper 也从已验证 bytes 加载。报告记录 snapshot manifest 与每份实际输入来源；工具链／标准库另由实际 Go 版本和可执行文件 hash 标识。

修复证明针对正常源码变更，不声称防御同 UID 恶意进程、编译器被替换或操作系统攻击。真实 GitHub 产物身份仍须另外核验，caller 提供的 SHA／run identity 本身不证明执行来源。

原红证据 source-binding-counterexample.json 保留。修复后的 test_build_uses_verified_snapshot_after_source_changes 只在外置合成副本改源，实际 Go 编译必须继续使用原六份 bytes 且无 sentinel。test_snapshot_bytes_and_inventory_rejected_if_changed 拒绝被改字节、额外和缺失文件。原独立五项回归不改断言。

永久 fast／symptom gate 均为 TestSwiftNameComparisonRegressionGate，由 scripts/check 的完整 Go 测试通过run-tests.py在全新独立checkout中调用全部28项主测试及独立 test*.py，包含实际 Go 编译与 CLI 路径，而非仅静态文字检查。测试源和输出全部明确标记 synthetic，不计为 Swift 互通。

没有发行受影响工具。回滚只移除独立对账工具、聚合入口与相关登记，不修改 oracle、分类 engine、reader 或用户资料。修复提交固定、原症状及受影响路径通过后才关闭；正式运行对账还须等待可验证的真实 Swift capture。

正式闭环：独立复审通过，正式scripts/check退出0（root race198.074秒），其中新独立checkout完整28项26.883秒、源hash不变且无noonmark-go相邻库；六目标编译及Linux隔离入口通过。原始红证据和五项独立回归保留，最新整合证据见docs/tasks/verify-swift-name-comparison/README.md。原“待commit”描述属于早期候选状态；本地修复不代表Swift语义对账已执行。
