# GO-RUNTIME-011：stream-safe NFC 碰撞静默吞掉标题编辑

日期：2026-11-06（新加坡）。本例修正 GO-RUNTIME-006 所选比较实现的边界，不撤回 canonical-equivalence no-op 的领域要求。

## 引入与红色证据

- 核心引入：`e5dd046ce45449d62ed68753d2851ce29656fcbc`，`domain-core/core/title.go`。
- gate 引入：`d24e2f369871781e67eb90f73bb193f9edbd402e`，`internal/inputdraft/gate.go`。
- session 引入：`1393bc49ed020787ffdbb35e26d239fac621c5a2`，`internal/titlesession/session.go`。
- `git log -S'norm.NFC'` 定位上述实际引入提交；Git author/committer 均记录 ferrumlux，但这是移植实施提交身份，不将代码缺陷归因于用户本人。
- 最小输入 a=31 个 U+0301；b=30 个 U+0301 + U+034F CGJ + U+0301。两者并不 canonical equivalent，真实 CGJ 不能忽略。锁定 x/text v0.43.0、Go1.27.3 实际选择 Unicode17 表，整串 NFC 会在 a 中自动插入 CGJ，导致两者比较相等。
- 原始纯 Go 探针输出 `tables=17.0.0 bytesEqual=false NFCEqual=true insertedCGJ=true`。核心两个入口误 no-op；Service 错误确认旧字节、不增加磁盘 revision；gate 将不同 owner 当作相同，允许错误 schedule/callback；session 吞掉草稿或将保存期间新输入错误标成 clean。均有候选修复前的独立红测试日志。

## 根本修复

共用 `domain/textcanon.Equivalent(a,b string) bool`，三个调用点统一使用。无效 UTF-8 一律 false，合法原字节相同可直接 true。每个合法 scalar 单独做完整 canonical decomposition，再对完整 nonstarter run 做稳定 CCC 计数排序；真实 CCC=0 scalar（含 CGJ）保持分段界限。比较时不改变任何存储或草稿字节。

不使用整串 NFD（同样会插入 CGJ），不删除 CGJ，也不只在 30 个码点的块内排序。固定 x/text v0.43.0，无 CGO、网络或新平台依赖。该库以 go1.27 build tag 选择 Unicode17；domain-core 最低 Go 版本同步根模块为1.27.3，避免单独构建时退回 Unicode15。依赖该模块的 domain-extensions/domain-codec 在独立 `GOWORK=off -mod=readonly` 下确认需要同步 go directive，故一并提高至1.27.3；没有更改依赖版本或 go.sum。穷举 Unicode17 全部合法 scalar 的永久测试证明：单 scalar 完整 NFD 的最长 nonstarter run 为3、无引入 CGJ、分解字节数不超过16，故 scalar-local NFD 不会触发30个 nonstarter 的 stream-safe 插入。表版本变更使测试失败，必须重新审查。

canonical decomposition + 完整 canonical ordering 构成 NFD；同一 Unicode 表下 NFD 相等即 canonical equivalence。CCC 取值至多256；每个非空 run 稳定计数排序保留同 CCC 的原顺序，时间 O(n)、空间 O(n)，无长逆序输入的平方插入排序。大小没有任意截断。UAX #15：<https://www.unicode.org/reports/tr15/>；stream-safe 属附加变换，不能充当丢弃真实字符的 equality 契约。

追加入口审查证实：core 两个标题更新入口原先接受非法UTF-8并改变graph（FF、尾截C3、截断E282、surrogate EDA080共8个红case）。现入口显式拒绝非法UTF-8并保持graph不变。Service同12分支原先已拒绝、无ack或写入；保留永久测试证明graph/revision/重开不变，不将其描述为曾经假确认。此范围不扩展AddPool等旧创建入口。

对合法文本，trim、CRLF、空标题、时钟、版本、能力守卫、持久化、输入生命周期语义不变。现有正常 canonical no-op 的完整原字节与 revision 测试必须继续通过。

## 永久门禁与限制

- comparator：`TestEquivalent`、`TestScalarDecompositionBound`，涵盖原始碰撞、29/30/31/32/60/61/4096/65536 边界、跨插入点逆CCC、同CCC稳定性、真实CGJ边界、Hangul、emoji、compatibility非等价、无效UTF-8。
- 核心：`TestTitleStreamSafeCollisionMustMutate`、`TestTitleInvalidUTF8UpdateRejectsWithoutMutation`。
- Service：`TestTitleStreamSafeCollisionPersistsExactBytes`，确认实际字节、revision、重开；`TestTitleInvalidUTF8UpdateRejectsWithoutPersistence` 确认无效编码拒绝。
- gate：`TestStreamSafeCollisionCannotRouteAnotherOwner`；非法UTF-8 owner fail-closed。
- session：`TestStreamSafeCollisionDraftIsNotSwallowed`、`TestStreamSafeCollisionNewInputDuringSaveIsRetained`；错误CGJ回包不能确认，正确回包才清理in-flight。
- 实际 MyGo Tester UI：`TestTitleUIStreamSafeCollisionSavesDistinctDraft`、`TestTitleUIStreamSafeCollisionNewerInputDuringSave`。
- 统一由 `scripts/check` 执行；本任务 `gates.tsv` 单行已合入正式总表，registry包含textcanon，不覆盖并行任务内容。

独立 Linux ICU76 对9001个共同字符输入对做NFD对账（固定种子），包括反例、随机长combining runs、Hangul与真实CGJ。它是独立规范佐证，不是 Swift/Apple oracle，也不宣称 Unicode16/17 全字符互通。

仅隔离 development/e2e 合成资料，外置 buildroot；生产、iCloud、Keychain、AI 均未启用。六目标编译不是各平台实机验收，headless Tester 不是操作系统输入法实机。修复不要求数据迁移；回滚代码会重新暴露静默丢编辑，应连同测试和三个调用点整体审阅。

修复实现提交：[`ab46ea6580fffd9cabb0e97b2a84b6c38c5a14f5`](https://github.com/ferrumlux/noonmark-go/commit/ab46ea6580fffd9cabb0e97b2a84b6c38c5a14f5)。正式完整门禁、六编译和Linux CLI均通过，详见[验收记录](../../tasks/fix-unicode-title/VERIFY.md)。
