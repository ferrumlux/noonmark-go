# Day 延期与延续接线验证

task-id: connect-carryover-ui

验证日期：2026-11-06。范围仅为隔离 development/e2e 的普通 Day 子集，尚不是全产品、Swift 互通或六平台原生运行验收。

## 来源与整合

正式基线为 462d6da。R7 的 35 项冻结清单 SHA256 为 `7576deb264386cc41de6e7a8122708a374267ff006eee895ccf2ad56464d7324`，独立审查后逐项导入；额外纳入 `internal/titlecoordinator/reviewer_retirement_test.go`，SHA256 为 `60228b6f10b6e54fd20edbaa14b8e688915f3860ca0215511577ae4477488a76`。原十份独立红例逐字保留，不弱化断言。原 Service、领域模块和六项固定 MyGo 补丁均未改动。

日期组件的默认入口保持既有行为，只有本批明确增加 pending variant。R6／R7 的自动标题退休改用精确 registration／session identity，无 IO、无自动重试，不复用或覆盖用户导航槽。已成功退休的事实必须按原 pointer／registration／identity 协调 UI，即使展示已被较新导航取代，也不能留下悬空注册或清掉新对象。

## 真实运行结果

- 作者完整 `scripts/check` 通过，root race 267.725s；独立完整复跑通过，root race 255.097s、taskapp 49.768s。
- 正式整合后的 `scripts/check` 退出 0：fresh root race 261.873s、taskapp 55.885s、titlecoordinator 1.028s、titlesession 1.515s。未改动的领域模块使用 Go 已有 test cache，不冒称全为 fresh。四模块 race／vet、固定 Unicode16 校验、51 项依赖负例及 native 门禁均通过。
- 正式 `scripts/cross-build` 退出 0，覆盖 macOS／Windows／Linux 的 amd64／arm64。它们是六目标编译，不是六平台启动。
- 正式 host build 与 `scripts/test-entrypoints` 退出 0：实际 Linux 二进制创建隔离会话、重开渲染 Day／Pool，并拒绝覆盖已有会话和无身份启动；没有桌面显示服务器，未运行真实窗口。
- 三张 1200×768 DIP／2× headless 图分别覆盖延期 sheet、历史延续 sheet、延期后的新日期，已逐张查看文字、选中圆、圆角及裁切。它们是单任务合成图，没有同页 Swift 像素 oracle，不能冒充一年中段互动演示。

正式运行证据位于显式外置 BUILD，`check.log`、`cross.log`、`build.log`、`cli.log` 各自附退出状态。测试资料不进入仓库或生产路径。

## 必须保留的门禁

同源未确认输入与无关链分别处理；writer 在途直接 native marked 输入、四阶段故障、publication 前 Close、先行 Complete／Return、稳定恢复 lease、午夜 Finish 失败、较新用户导航、成功退休后重入以及替换 owner 均有自动测试。coordinator 另覆盖 native Finish 重入 navigation／pending close／pending quit／新注册及整批 clean 复验，无部分退休、扩集合重试或自动 IO。

合法 Changed-hook 的 app／coordinator 合同红并不是已证实的 OS 事件路径；文档与测试不将它误称为系统 IME 缺陷。故障案例 GO-RUNTIME-CARRYOVER-UI-001 以修复提交 `961e688dcf0077c7976a61e6f5e1279ef6254960` 在上述受限范围内闭环。首次仅文档复核命令误用 `TestFailureCase`、没有匹配测试，未计为通过；改用精确 `TestFailureGateRegistry` 后真实执行通过。

## 保留边界

- 当前保存中仍使用原「取消」标签，其语义仅关闭展示，已准入写入继续；目标已冻结，不能把关闭说成撤销。保存中提示文案另行改进。
- snapshot undo、Continue 对 undo 历史的完整失效、成功 toast、完整 Unfinished 页面和 locale 尚未对等。
- 仍是严格空 catalog 子集；没有任意外部图删除后的 ghost-parent 恢复或生产数据兼容声明。
- 尚无一年中段 Day UI、实际 Swift、macOS／Windows／ARM64 原生窗口与系统 IME 证据。既有 365 天 Service 性能 probe 不是原版全年演示故事。
- 本批未作对应远端 CI、正式打包或发布声明。此前 462d6da 的本地候选可复现结果不外推到此修改。回滚只撤入口，不能删除或降级已经确认的 deferred／continued 事实。
