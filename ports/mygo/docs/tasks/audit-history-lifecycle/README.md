# 未接线 lifecycle 精确历史审计

task-id: audit-history-lifecycle

日期：2026-11-06。风险：P。状态：作者、独立复核及正式全项目门禁均已通过；仍未接线。

正式来源 HEAD `3ea3efeaee2a0085576ca1aadd0d3082e643a558`，tree `b7097068ebf44c167b6df8ac9162261946ea3bfb`。新副本由精确 committed files 导出；不复制 toolchain/cache，不改正式树或上游。隔离移植授权取代上游针对另一主机的固定路径历史要求。

本任务只允许两个 package-private typed Abandon／Reactivate 审计。不是 Service、store、UI、v3 writer、全局 frontier 或持久写权限。详见 [设计](DESIGN.md) 与 [验证](VERIFICATION.md)。


## 实施范围

新增 `history_lifecycle_command_vnext.go`；旧 `history_command_vnext.go` 仅扩充两个 kind、effectiveTrace/eventID scalar 与显式 valid switch。两个旧 AST 测试增加新入口及 ID gates，原断言保留。原 498 tracked 文件中 495 个逐字节及 mode 相同，其余恰为上述三个文件；旧 audit bodies 与 bind/match/finish 整段 byte-exact。这是作者隔离阶段的范围对账；现已把冻结补丁及独立测试整合至正式工作区，另外只补任务文档和移植 README 入口。

Abandon 保留请求与实际 target 身份；Reactivate 根据 owned prior 的日期／锁状态派生 P 零 capture 或 U 单 capture，旧 A 保留。新 ID 可规范化，但 proof eventID 保存 caller 的原字符串。caller slice 不进入结果。所有成功必须完整 replay＋inspector＋prefix；未知 kind 与失败零结果均拒绝查询。

## 测试边界

本批来源与 adversarial 测试检查完整 graph/supplement、显式 event/status/trace/At/revision、proof scalar、独立 copies 和失败不变。包括 historical 请求的 pending 路由、wrong owner、同链合成多 A 双分支、无 lineage 多 U 参数例、current/future/past 与 locked 分支、open U silent settlement、多轮 A/P 与原 completion/undo/settlement、rich content、exact clocks、预算次序、并发只读与 future X+A 拒绝。

真实 continuation 链的前一 U 不能直接改 A：后代引用要求来源为 U/D，core 会报 invalidTraceContinuation。首次 focused 的 oracle 错误已保留为精确失败测试，并另建无 lineage 的参数例证明“无 pending 时选请求 U”；没有放宽 validator，也未宣称任意多 U 组合均可成功。多 A 例不存在 latest-A 限制，均从同一 immutable prior 分别执行。

保留全部失败证据，详见 VERIFICATION。工具只使用既有 warm cache、固定依赖与显式外置 build；生产／home／iCloud／Keychain／live AI／GitHub 未访问或改写。


## 独立准入与正式整合

独立最终生产字节已通过 focused 0.188秒、完整taskapp 29.774秒、完整未过滤race 178.377秒和vet，详见 INDEPENDENT-REVIEW。独立方未读取或导入作者新测试，其711行新测试已按冻结SHA整合。原498文件中仅共享结果元数据、两个AST测试与移植README四个文件变化，其余494文件字节与模式保持。旧三个command入口主体及bind/match/finish整段仍逐字节相同。

正式整合的全项目检查、六目标编译、Linux构建及CLI均已实际通过，详见以下精确整合记录。仍不接Service/store/UI，不扩大v2或fixed profile；无新Swift、真实桌面、Hosted CI或发版声明。


## 正式整合结果（2026-11-06）

合并后 focused old+new 检查退出0（7.079秒）。随后同一冻结树从05:12:22 UTC顺序运行：

- `scripts/check`：退出0。主UI/race 464.273秒、taskapp/race 179.122秒；其它未改动包部分使用既有cache，完整日志区分cached与实际运行。MyGo UI 2.656秒、popup完成检查1.054秒均通过。
- `scripts/cross-build`：linux/windows/darwin × amd64/arm64 六目标全部编译成功，退出0；不是目标实机运行证明。
- `scripts/build`：Linux构建退出0。
- `scripts/test-entrypoints`：退出0；开发身份、创建、重开、目录复用保护以及Day/Pool两张2400×1536 MyGo渲染通过；未测试真实桌面窗口。

511个仓库文件在整轮门禁前后字节及模式完全相同；九份Go文件与作者/独立冻结SHA一致。随后仅回填本README的实测状态，不改任何Go文件。精确命令、起止/退出码、完整日志和七个二进制哈希保留于外置整合证据目录。没有把产物、工具链、真实资料或缓存加入仓库。

独立复核及正式整合验证仍不使这些helper具备持久化写权限；whole-consumer、live-head、no-op/disk-prefix和新development clean cut仍须单独设计/接线/验收。本批没有新GitHub写入或Hosted CI，旧工具审批结果未明而暂停，不以替代接口或重试绕过。
