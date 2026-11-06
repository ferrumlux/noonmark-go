# 手动进度领域与开发 Service

## 需求与现状

Task ID：connect-manual-progress。只在隔离研发副本实现普通任务的上游 `setManualProgress` 语义，不改 UI、reference、MyGo 或正式工作树；不发行、推送或访问 production、iCloud、Keychain、AI。源基线为公开上游 `43e6eef`，Go 起点 `ee6e484`。现有 immutable `ProgressProjection` 已提供历史 floor 与能力查询；`DayTrace.ManualProgressPercent`、clone、v7 codec 和范围 validator 已存在，但没有写入命令。Service 的通用 capture 不能用于任意历史改写，须增加封闭命令重放。

## 源语义

核对 `NoonmarkEngine.swift:2842–2864,3937–4020`、`Models.swift:1037–1054` 及 `TrajectoryTopologyValidator.swift`。错误次序为 trace 查找、locked day、今天且 pending、整条 chain 的实际 presentable child 历史、clamp/floor、optional 相等 no-op、时钟。过去／未来／非 pending 均 immutableHistory；任何 formsDayHistory trace 的非 cancelledDraft child 阻断（包括未来和历史），planned children 不计。输入整数先 clamp 到 0..100，再取历史 floor；当前值可下降到 floor。nil 与 0 不等，100 不自动完成。no-op 不检查 now，也不更新时间。非 no-op 接受与旧 content clock 相等的有限时间，拒绝倒退或非有限，且只修改 trace 的 manual percent 与 contentUpdatedAt。

Go 现有强类型边界额外拒绝无效 LocalDate；明确 chain+trace 路由不匹配返回 ErrInvalidInput。Swift invalidInput 的时钟原因沿用 Go ErrInvalidClock，不扩大 kernel 的 recurring 支持。

## 实施设计与边界

- 新增 `core.SetManualProgressInput` 及 `TaskKernel.SetManualProgress`，复用同一个 progress projection 算法；`mutate` 仍 deep clone、完整验证后发布。
- Service 新增 `SetManualProgress(chain, trace, percent) error` 与 `SetManualProgressAcknowledged(...) (int,error)`。同锁解析明确 owner，执行命令、审计、v7 规范化、磁盘确认、发布。确认值仅来自规范化候选，同值 no-op 仍检查完整磁盘 prefix。
- Service 先以目标 trace 当前 content clock 运行私有候选判定 no-op；只有实际变化才调用全图单调时钟并从原图重放，避免 no-op 因无关时钟 frontier 耗尽而失败。不会发布探测候选。
- 专用审计重放封闭命令，完整比对 before/prior 与 after/expected；独立冻结全部其他 section、其他 trace，以及目标 trace 除两项写入字段外的所有字段。保留完整 empty catalog 历史、runtime scope、codec、validator 约束，不增加字段豁免 API。

## 风险、回滚与验证

涉及持久化，按 P 级收敛为研发身份。无迁移、无新 wire 字段或 schema；撤回本新增入口即可回滚，已经合法写出的手动字段可由现 reader 完整读取，不覆盖已有会话。无灰度发行或生产监控，风险监测只靠隔离测试和四阶段故障注入。

永久自动化测试覆盖真实领域命令合成 fixture：clamp、nil/0、no-op 时钟、全 chain child／取消墓碑、历史 floor、撤销完成、回池排期和跨日；Service create→set→reopen、四阶段故障、并发/冲突、精确确认、no-op prefix 损坏检查和精确审计篡改拒绝。执行 core/codec/extension/app 全集 race+vet 与聚合 check；旧测试不删改或 skip。若发现旧 bug，先保存 red 证据，再写独立故障案例和 fast/symptom gates；未确定修复 commit 前保持处理中。Swift oracle 和 macOS/Windows 实机不在此环境，不宣称已验证。

## 极值重开安全修复设计
独立原红例已在本副本重现：合法 MaxFloat64 内容时钟落盘后，同日 Open 在无变化时提前 nextInstant 溢出。修复将持久化 clock 集合抽为同一只读来源；结算先以已有最大有限 frontier 执行私有预检并经过完整审计与规范化比较。空图以合法零 Instant 预检，不是写入时间。只有实际变化才采样 wall clock、分配严格递增时刻并从原图重放。预检不发布、不提交；真实变化仍完整验证与原子提交。AdvanceTo 不再因 today 相等跳过，no-op 必须检查 closed、blocked 与磁盘完整 prefix。测试保留原始红断言，覆盖极值实际结算拒绝、普通结算、未锁历史、空图、损坏／冲突及四阶段故障。禁止放宽 validator／codec／历史审计。
