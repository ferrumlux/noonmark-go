# 分类领域第二批移植

来源：`Sources/NoonmarkCore/TaskClassification.swift`、`TaskClassificationStateValidation.swift` 和 `Tests/NoonmarkCoreTests/TaskClassificationModuleTests.swift`；基线 `43e6eefaae3a04be89cab3e234d470111afd80ef`，AGPL-3.0-only。

本包是独立的低层领域组件，不是 NoonmarkEngine 的替代品，不接入 production、iCloud 或 Keychain。

## 已实现

- 强类型 `CategoryID`、`LabelID`，完整 `Source` 联合字段与 `Source.Validate()`；拒绝混入其他 source 的关联字段。
- `New`、`Engine.Execute`：在锁内验证 expected revision、互动幂等、有限时间和来源，先复制后发布。失败不会发布历史或部分状态。
- 建立分类／标签、名称版本连续区间、保留旧名别名、重命名 no-op、归档／恢复、合并链、合并时保留已有目标关系的来源、删除未引用实体与 tombstone。
- 当前一个 category 加多个去重 label；移除关系保存完整 origin source、decision、时间、revision；现存归档关系可保留，但不能新增归档关系。
- 显式 `Capture` 追加 trace 历史值，后续改名／合并不改写旧快照。
- `ResolveName` 解析当前名、旧别名及合并终点；不会静默建立实体。
- `Snapshot` 返回深复制；`RestoreSnapshot` 重放所有命令，逐字段核对派生状态并检查 core graph 引用。该格式仅属于本 Go 包，不是 Swift Codable 格式，也不是防恶意管理员的签名审计日志。
- 生成的历史／名称事件 ID 由 interaction 与事件用途确定；输入分类、任务、trace 和 interaction ID 原样保留。

## Unicode 边界

必须注入固定版本、确定性的 `NameCanonicalizer`。生产 adapter 必须实现 Swift display whitespace 规则及 ICU NFKC_Casefold，调用者不得注入 lowercasing 冒充兼容实现。当前仅附 `FixtureCanonicalizer`：只接受测试登记的精确输入，不支持的输入明确返回 `ErrUnsupported`。fixtures 包括全角 ABC、Straße／STRASSE、soft hyphen；未宣称完整 Unicode 兼容或已运行 Swift 差分测试。adapter 及其输入表属于可信、不可并发修改的配置。

## 尚未移植与不能切换的边界

- Swift prepare/confirm capability、plan digest、用户确认令牌、Zhulong / automatic AI authority、pending presentation approval 工作流、undo/redo 与自动 job generation。命令直接拒绝 AI、Zhulong、Inherited 来源，不能借本包绕过授权。
- 完整 Swift Codable 导入／导出、existing-item reuse notices、catalog projections、canonical audit-array ordering 与跨设备 reconciliation。
- 分类删除时“解除当前／未来任务关系，保留历史”的高层多次 commit 编排；这里只实现未引用实体 hard-delete。
- 任务 copy/change 的继承、day settlement、return-to-pool、reactivation、abandonment 自动捕获钩子，以及与 task/recurrence 的共同事务。本包不修改 core graph；Capture 只核对 trace 身份、状态和时间，调用者仍须决定何时冻结历史，尤其不能提前冻结今天可撤销的 completion。
- 为 fail-closed，本包比 Swift 的按实体 mutation clock 更严格：所有新命令必须使用不早于上一命令的时间。没有实现允许无关实体回溯时间的完全等价行为。
- 无 HTTP、SQL、sync 或 UI 集成，无部署验收；仅有合成领域测试，不可据此宣称全功能迁移完成。

## 验证与上游追踪

`classification_test.go` 各回归测试注释指向上游同名测试族。覆盖名称版本／别名、旧历史冻结、归档关系、新关系限制、合并去重与 origin、合并链、硬删除、反向时钟、非有限时间、未知 command 字段、互动重试／冲突、source 联合验证、并发 CAS、快照隔离、重放篡改检测和丢失 graph 引用。

运行：`GOCACHE=/tmp/noonmark-class-cache ../go/bin/go test -race ./classification`；`../go/bin/go vet ./classification`。完整应用路径、Swift differential 与持久层事务未运行。

显示名rename的Swift规范等价no-op现使用共享textcanon，不与NFKC_Casefold key相等混用。旧byte-equality错误记录会被严格重放拒绝，不自动重写。边界及永久fixture见[GO-CLASS-001](../../docs/engineering/failure-cases/GO-CLASS-001.md)，完整Unicode规范器与runtime接线仍未实现。
