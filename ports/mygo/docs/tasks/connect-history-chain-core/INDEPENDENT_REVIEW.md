# 普通任务链独立审查计划

## 需求与现状

只审查 frozen 四文件补丁，不修改作者副本、正式 pending 改动或上游来源。正式基线为 `7c239df`，Swift 来源为 `43e6eef`。独立副本由正式 HEAD 的 Git archive 加已校验补丁建立；四文件 SHA-256 全部匹配。只允许合成领域资料，不做远端、production、iCloud、Keychain、AI、release 或全量 aggregate。

## 设计与验证

先静态核对 Swift abandon/reactivate、ContentMutationClock 与 continuation topology。另写 external test package，仅用公开 API 与自行定义的 fixture，不调用作者新增测试 helper，也不重复实现生产命令。测试断言覆盖：错误优先级及原子回滚、多段真实续接的历史请求重定向、被引用历史源的拓扑拒绝、合法 civil date 两端、随机有限 IEEE 时钟及保留位模式、输入/输出深复制、并发读写一致性。

运行固定 Go 1.27.3 的定向 race、domain vet 与既有 reader 的 abandoned audit 拒绝；日志、TMPDIR 在外置 `/workspace/shared/noonmark-history-chain-independent-20261106`，共享既有缓存。不会用 Go 自洽或源码静态对照宣称已跑 Swift oracle、Service、持久化、UI 或跨平台实机。

风险与回滚：新增测试仅留独立副本；不更改生产行为或 validator。发现源语义差异立即报告，不松断言、不替作者修复。最终记录实际命令、结果及剩余边界。

## 独立审查结论

本批可按已声明边界整合为未接线纯核心候选，未发现阻断缺陷。不是完整 Swift engine / App 对等批准。原四文件没有改动，独立副本仅再新增本报告及 `domain-core/core/history_chain_independent_test.go`。

已独立核对：

- `NoonmarkEngine.swift:2513–2572,3814–3819`：请求 identity 绑定后，仅同链 pending 作为实际目标；无 pending 时才用请求源。没有引入日期、锁日或最新历史前沿资格限制。Go 的额外 owner fence 属已声明 API 围栏。
- `Models.swift:1042–1053`：trace 和 chain 均按有限非倒退内容时钟处理，不使用 max/touch 替代。相等可以写入原始 Now 位模式。
- `NoonmarkEngineTests.swift:1801–1898,3868–3891`：原 ID、开放子项、历史源、锁日优先及自然日倒退与候选一致。源码没有修改 note、definition、manual progress、子项或身份；作者 whole-graph 断言与独立 snapshot 隔离测试均通过。
- `TrajectoryTopologyValidator.swift:228–246`：abandoned 不可作为 continuation 源。真实公开 Go 命令构建三段链并完成尾段后，对前两个被引用 unfinished 祖先执行 abandon 均被既有 validator 拒绝，完整回滚。这是 Go 发布保护，不能宣称 Swift abandon 命令本身有相同错误顺序或已实跑得到相同结果。
- `TaskClassification.swift:4022–4059`、`TaskClassificationModuleTests.swift:1880–1923`：abandon 与 unfinished reactivate 必须追加真实 capture，pending reactivate 保留旧事件/latest。本批不实现它们；现有 reader 仍拒绝 abandoned audit 与非空 catalog。

## 实际运行结果

工具链：`go version go1.27.3 linux/amd64`。全部正式复验命令退出 0：

1. domain-core：`go test -p=1 -race -count=1 -run '^Test(ReviewerHistory|HistoryChain)' -v ./core`，20 个顶层测试（独立 7、作者 13），1.173 秒。日志 `logs/independent-and-author-race-2.log`。
2. domain-core：`go vet -p=1 ./...`，退出 0，日志 `logs/domain-vet-1.log` 为空。
3. 根模块：`go test -p=1 -race -count=1 -run '^TestEmptyHistory' -v ./internal/taskapp`，1.531 秒。明确包括 `TestEmptyHistoryRejectsCorruption/unsupported_status`（abandoned）及非空 catalog 拒绝。日志 `logs/reader-history-race-1.log`。
4. 根模块：`go test -p=1 -count=1 ./internal/taskview`，0.014 秒。日志 `logs/history-projection-1.log`。
5. domain-codec：`go test -p=1 -count=1 ./v7 ./canonical`，分别 0.168、0.008 秒。日志 `logs/codec-compatibility-1.log`。

独立测试额外覆盖 11 个错误优先级组合、三段续接后原位重新启用再接第四段、打乱 trace slice 顺序、较新的历史源时钟、两个被引用祖先的失败回滚、518 个确定性有限 IEEE 时钟样本及相邻倒退值、0001/9999 civil-date 端点、闰日、旧快照跨完整生命周期仍不变、constructor/输出指针不泄漏，以及 10 个 goroutine 共 1,000 次混合 abandon/reactivate 与同步 snapshot 的完整发布。

首轮 `logs/independent-race-1.log` 有一项红灯：独立审查 fixture 写成 `PinOrder=0`，在进入命令前被既有 `invalidTraceStatusFacts` 拦截（validation.go:276）。该字段必须大于 0。只将自建 fixture 改为合法的 1 后复验；未修改候选实现、validator 或行为断言，首轮日志完整保留。其余六个独立测试首轮通过。

## 保留边界

- 未运行 Swift 测试、跨引擎 oracle、macOS/Windows/ARM64 实机、UI、IME、读屏、Service 持久化、release 或全 aggregate。
- 后续 Service 不能直接用成功 purecore 图替换完整 snapshot。必须在同一事务捕获实际目标的分类历史、精确审计 before/after、保留全部 supplement、核验锁日与时钟，再单独证明 reader/codec/磁盘恢复。
- 原作者四文件最终哈希仍匹配 frozen 清单。正式仓库既有 pending 改动保持原样。没有 commit、远端动作、production/iCloud/Keychain/AI 访问，也没有清理缓存或复制工具链。

本报告保留整合前的独立验证过程；随后正式组合验收见同目录 VERIFY.md，不能把本报告历史时点当作当前未整合状态。
