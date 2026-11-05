# 标题编辑核心第一阶段

2026-11-05。新增 `domain-core/core/title.go` 与 `title_test.go`，并为 Unicode canonical equivalence 更新 domain-core 的 go.mod/go.sum；没有接通 service、持久化、分类审计或 UI，也没有放宽 `CaptureEmptyHistory` 或运行时资料范围。

## 实施边界

- 强类型 `EditTaskTitleInput` 注入 ChainID、Title、Today、Now、NewDefinitionID。两个入口为 `RenameTaskTitle` 与 `SaveTaskTitleInput`。
- 保留 active、可编辑事实、pending 日期和锁日检查顺序；能力检查先于 normalized 相同标题的 no-op。Today 必须为有效日历日期。
- 同标题比较使用 x/text/unicode/norm 的 NFC canonical equivalence，与 Swift String equality 语义对齐；只归一化比较值，原存储字节及真正新标题的输入字节不做 NFC 改写。不得改成 NFKC 或 case folding。
- 同标题不使用 Now 或新 ID；分叉以外不校验、不保留、不生成新 ID。标题 CRLF/CR 转 LF，随后使用现有 Go Unicode trim 契约，可清空已有标题。
- rename 在有日事实时分叉；saveInput 仅在当前定义仍被非 pending 日事实引用时分叉。cancelledDraft 不计日事实。原位保存不推进 trace clock。
- 新版本 maxSequence+1，复制 description 与 planned subtasks，并与旧版本保持独立内存所有权。旧 current 只改变 supersession metadata/contentUpdatedAt；全部 pending 重绑，其他 trace、day、subtasks、notes 不动。
- restoration witness 不是禁止编辑的理由：原位保留，分叉用本次 successor 替换。已 superseded 版本不改。
- 与上游 Models.swift ContentMutationClock 一致，变动 definition/trace 的 Now 必须有限且不早于自身 clock，允许相等；NoonmarkEngine.touchChain 使用 max(updatedAt, Now)，不能因为 chain clock 较新额外拒绝。服务层之后仍应提供完整 frontier 的 NextMutationInstant。
- 沿用 mutate 锁、候选深拷贝、ValidateTaskGraph、成功后发布；失败不发布部分标题、版本或 trace。

## 验证

在 domain-core 使用外置 Go 工具链、GOCACHE/GOPATH，并限制并行及内存：

- `go test -p=1 -race ./...`：通过。
- `go vet -p=1 ./...`：通过。
- 独立 canonical-equivalence overlay 回归：修复前两个入口均判红，修复后 race 全套通过。稳定回归入口为 `TestTitleCanonicalEquivalenceNoOp`；覆盖双向预组/组合字符、Hangul、组合顺序、无效 clock/ID 不消费与字节保留，另测能力 guard 和 compatibility 字符不能误合并。
- 官方依赖 `golang.org/x/text v0.43.0` 与根模块版本一致；该依赖要求 domain-core Go minimum 从 1.23.0 提高到 1.26.0，当前验证工具链为 1.27.3。依赖从已有本机缓存加载，未做网络下载。

新增测试覆盖池任务/今日/未来两个入口、精确字段差分、continuation 首次分叉后二次输入复用、重复显式 rename、returned 历史与 notes、未锁 completed 历史引用、cancelledDraft、核心清空与创建拒绝、CRLF/CR/中文/Emoji/组合字符/Go Unicode whitespace、no-op 不消费无效 clock/ID、能力 guard、时钟回退/非有限/相等/最大有限值/较新 chain clock、同链及跨链 ID 冲突、sequence 溢出与未消费路径、witness/maxSequence、非当前版本 pending 重绑、深拷贝、并发以及无效候选拒绝原子性。

官方测试对照来自只读 `NoonmarkEngineTests.swift:74–250` 与 `NoonmarkEngine.swift:1216–1322`、`3556–3568`、`3805–3812`、`Models.swift:1042–1053`，没有运行 Swift oracle。所有资料为合成 fixture，没有 production、iCloud、Keychain 或 AI 网络调用。

## 尚未验证或不可宣称

- 既有 ValidateTaskGraph 拒绝同链多个 pending（duplicateActiveTrace）。实现遍历全部 pending，但该矩阵只能验证构造器/无效候选拒绝；没有修改该不变量，不宣称多 pending 图已获支持。
- Foundation whitespacesAndNewlines 与 Go TrimSpace 字符集没有真实 Apple oracle，不能宣称 Unicode 归一化完全对等。
- witness、完整快照差分等目前仅有静态上游证据与 Go 合成测试，不是 Swift 执行对账。
- 专用历史审计、故障/重开/竞争持久化矩阵、IME/700ms autosave/退出 flush 均属后续阶段。当前 UI 仍不得因核心通过而开放标题编辑。

回滚：本阶段没有运行时接入，撤回新增核心文件及其 x/text 模块依赖即可；不迁移任何用户资料。灰度与监控：本阶段不启用 UI 或服务开关，不发布用户可见编辑能力；后续集成必须先通过专用差分审计和持久化症状门禁。
