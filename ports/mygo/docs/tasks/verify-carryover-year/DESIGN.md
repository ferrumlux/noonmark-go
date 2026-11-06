# 一年中段 Day 延期与延续验证

task-id: verify-carryover-year

## 需求与现状

这是冻结 R7 之后的独立测试批次，不改变 R7 的 35 个冻结文件，也不新增业务功能。现有 manualServiceYearFixture 只有过去 364 日至今日，已有单任务 native Tester 证据不能证明一年中段的实际 UI 路径。

基线为正式 462d6da 加 R7 冻结候选，manifest SHA-256 为 7576deb264386cc41de6e7a8122708a374267ff006eee895ccf2ad56464d7324。正式仓库及 Swift 43e6eef 只读。源码在独立 sibling noonmark-go-carryover-year-review；所有资料、日志、图片与构建产物位于全新外置 BUILD。

## 合成范围

- 固定 today=2026-11-05，前后各 182 日，共 365 个连续日期。每日期两个任务的首轮 730 任务子集已通过；按追加验收要求，最终 fixture 在 today 额外加入 32 个 pending 普通任务，共 762 个任务，today 共 34 个。过去每日期一个完成、一个未完成；今日一个完成、33 个 pending；未来每日期两个 pending。全部 chain 无 cycle membership，catalog 为空，没有子任务。
- 按日期通过真实 TaskKernel CreatePoolTask、ScheduleFromPool、MarkCompleted 建立，再通过 SettleTaskDays 结算历史。未来任务以固定 today 排期，不能把未来当作已过去。
- 仅 internal/taskapp 的 `_test.go` 可使用与既有 benchmark 一致的隔离 aggregate 安装：CaptureEmptyHistory → normalizeSnapshot → disk.commit。它不是正式导入器，不手写磁盘 JSON，不跳过 schema/history/domain 验证。Close 后以公开 Service.Open 重开并对账。
- root 测试调用独立 test-only fixture 测试生成一个全新 e2e session；没有新增生产 API 或测试逃生口。普通完整测试也执行 fixture 自检，不通过 skip 隐藏启用失败。

## 实际路径与断言

root 测试以 native Tester 的真实右键菜单、日期格、确认、前后日及回到今天按钮进入 Service writer FIFO。test-only forwarding wrapper 仅计数和计时，所有业务调用仍由真实 Service 执行。

先验证今日及历史菜单边界；日期草稿与取消不得写入。今日 pending 延期至现有未来日，重复确认只能执行一次，ack 新 ID 必须不同于原 ID 且 UI 选择/日期与磁盘一致，原 trace 保留为 deferred。然后返回历史未完成源，验证只读 UI 与无写入导航，延续到今日或未来；原源及历史日完整不变，只新增一个正确新 ID。完成任务和未来普通任务不得出现不合法 carryover 菜单。命令前后完整历史切片、无关任务及文件签名对账，不只检查任务数量。最终 Close/reopen 再通过真实 Day UI 浏览历史/今日/未来，确认无自动重复命令。

截图直接保存 Tester.Image 原图并实际看像素；不拼接、不制造 Swift 原版截图。命令日志保留首次失败及原断言。若出现业务红，先报告与留证，不修改业务来使本次验证变绿。

追加长列表验收：使用真实 viewport 验证今日末行开始时不可右键命中，再以 Tester.Scroll 使其可见，通过该行原生菜单打开日期弹层并确认。比较滚动前后固定日期条位置，检查 modal 仍在窗口内，保留滚动前、滚动后及弹层原图。不能以任务数量代替实际滚动与命中证据。

## 风险与结论边界

这是 development/e2e 身份的受限 empty-catalog 合成负载；不用 production、iCloud、Keychain 或真实 AI。headless native Tester 不是 OS App、WindowServer、真实系统 IME 或 Swift 像素对比。一年连续日期不等于原版完整一年用户故事、跨季度功能重复使用、SQLite、加密烛龙 sidecar 或完整 demo 对等。今日 34 行仅验证本次滚动与弹层，不外推任意规模列表性能，也不声称跨全年日期均有实际 UI 交互覆盖。

本批只交付新增测试、设计、命令及运行证据。整合阶段单独整合 R7，本批不能冒充 R7 既有门禁或已经合并。回滚只移除新增测试资料，不改正式业务或既有用户资料。禁止 GitHub 写入、commit、release 与清理共享 cache。

## 默认门禁与复跑入口

- `internal/taskapp/carryover_year_fixture_test.go` 的 `TestCarryoverYearFixture` 默认生成全新 `t.TempDir()` 会话并完成 Close/Open 自检；显式 `NOONMARK_CARRYOVER_YEAR_SESSION` 仅供 root 测试导出到它刚建立的全新路径，路径已存在即失败。
- `runtime_carryover_year_test.go` 的 `TestCarryoverYearMiddleNativeService` 每次新建目录，调用当前 Go 工具链的独立 fixture 测试，成功后才公开 Open。既有 session 不被复制或沿用，生成失败、丢失、打开失败一律 `t.Fatal`，没有 skip。环境变量仅选择保留证据的目录，不提供 fixture 内容。
- 两个测试已位于默认 `go test ./...` 包中。现有 `scripts/check` 第一模块的 `go test -p=1 -race -buildvcs=false ./...` 会执行 root UI 测试以及 internal/taskapp fixture 测试，无须增加生产入口或改冻结门禁。root 所启动的 fixture 子进程为常规 Go test；internal/taskapp 包在全量 `scripts/check` 中的独立运行则带 `-race`。必须据实际日志区分，不能把父进程 `-race` 自动算到子进程。
- `scripts/test-carryover-year` 是可选保留证据入口，使用现有严格 `scripts/environment`；每次建立全新外置目录，主 UI/Service 测试带 `-race`，保存实际 PNG、development session、命令日志。成功后生成该轮 SHA256SUMS；失败时保留已有原始资料并返回失败，不声称已自动生成 SHA。session 不进入源码、Git 或 CI artifact 合约。

## R2：fixture 子进程的明确生命周期

独立审查确认 R1 的 `exec.Command(go test)` 与父测试共享默认十分钟上限，父退出不等于 go driver 的测试子进程退出。本次修正仅测试设施，不修改原业务断言、领域或 UI。R1 原源码及证据完整保留。

先以有界 Context 执行 `go test -c`，将 fixture 测试二进制写入本轮新建外置目录；编译成功后才直接运行该二进制的 TestCarryoverYearFixture。真正持有 development session 的测试进程因此是直接子进程，取消后等待它被 Wait 收束再返回。编译阶段尚不打开 session；取消 go driver 不外推为其临时 compiler 后代全已结束。所有阶段使用同一预算，不重新延长 deadline。

预算来自父 t.Deadline，预留15秒给返回与清理；无父 deadline 时使用固定8分钟安全上限。预算不足即失败且不启动子进程，不 skip。每条命令设置有限 WaitDelay 防管道等待无界；fixture 自身 -test.timeout 早于 Context 截止，父 Context 取消亦传递。不得抬高默认root十分钟门禁。

新增快速真实 helper 进程回归分别验证正常输出、非零失败、取消与到期后直接子进程 Wait 收束，以及已取消 Context 不启动进程。它们不访问业务 session。完整年度实际路径还需在最终R2源码定向race，再由整合阶段组合执行聚合门禁。CommandContext 和 WaitDelay 不被描述为跨平台进程树杀灭。
