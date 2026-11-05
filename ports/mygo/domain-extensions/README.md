# Noonmark Go 领域扩展：分类、重复计划与历史

第二批可合并的领域实现，独立于 UI 原型。来源基线：`quboliu/noonmark@43e6eefaae3a04be89cab3e234d470111afd80ef`。遵循上游 AGPL-3.0-only，保留 `LICENSE`、`NOTICE` 和源文件标识。

## 目录与依赖

- 本模块：`github.com/ferrumlux/noonmark/ports/mygo/domain-extensions`。
- 依赖第一批：`github.com/ferrumlux/noonmark/ports/mygo/domain`，开发时 `replace => ../domain-core`。
- `classification/`：低层 catalog 与关系历史、命令执行、显式 trace 快照、重放验证。
- `recurrence/`：重复计划事实、创建／转换／修订／跳过／停止／完成目标结算、模板内容／分类版本和轨迹投影。
- `integration/`：仅测试。展示在私有 candidate 上同时准备父模板和子任务分类，最终验证后才发布，以及为什么删除分类必须检查父模板全部历史版本。
- 本层使用标准库与 core，间接依赖 core 的锁定 x/text；Go 最低 1.26.0。不依赖 MyGo、HTTP、SQLite、CGO 或平台 UI。

没有修改第一批、Swift 参考仓库或 `prototype`。没有读取、定位或使用 production、iCloud、Keychain 或实际个人资料。

## 重复计划接口

`recurrence.State{Tasks core.TaskGraph, Series []Series}` 持有现有任务图及重复父实体。所有命令先验证、深复制、修改 candidate，再整体验证；失败返回原状态，不发布部分事实。`IdentitySource` 注入 UUID 生成器，支持确定性测试；默认使用第一批的安全随机 UUID。

- `State.Create(CreateInput, IdentitySource)`：独立 occurrence chain / definition / trace，复制子任务 lineage。
- `State.Convert(ConvertInput, IdentitySource)`：采用原 task-pool chain；若已排期，保留恰好对应第一个 occurrence 的原 trace。新的父模板不自动继承原任务分类，遵循当前上游行为。
- `State.Revise(ReviseInput, IdentitySource)`：已开始计划仅从明天生效；未开始计划可调整起点。保留全部旧 revision 和 occurrence；重新加入先前由 planRevision 取消的日期时采用原 chain、追加 definition 和 trace。用户主动跳过的日期不复活。
- `State.Skip`、`State.Stop`：只处理允许的未来 pending occurrence。今天的 occurrence 延期至未来，仍不被 Stop 取消。取消保持 trace 身份、取消 witness 和日期，不删除历史。
- `State.Reconcile`：完成目标仅统计已锁定、早于今天的 completed chain。达到目标后，以已确认完成日期的最大值停止后续 occurrence，今天尚未执行的目标也可取消。
- `State.UpdateContent`：只更新 today/future editable member。定义有历史引用时追加新版本，不重写历史；子项取消保留 identity、lineage、位置和 witness，重新启用同 lineage 时恢复原子项。
- `State.SetTemplateClassification`：仅修改父模板 revision ancestry；终止计划也允许父模板分类变更。不会偷偷修改成员的分类。
- `State.EditableMemberChainIDs`：提供模板分类传播范围。
- `State.Validate`：第一批任务图约束 + 父实体存在、每个 occurrence 至少一个 trace、同父同日期唯一、membership 日期集合严格等于所有 plan revision 的历史并集、分类 ancestry 无环且有单一根、稳定事件排序和有限时间。
- `State.ValidateCatalogReferences`：必须由完整快照 owner 调用，检查所有父分类 revision，不只是当前值。
- `State.Track` / `Tracks`：保留 off-schedule 日格、延期及继续目标、completed 优先于 unfinished、跳过无导航、active/upcoming/ended/stopped 生命周期及原排序规则。父 terminal fact 不会因本机日期回退失效。

计划窗口最多 366 个日历日；每 N 天支持 1–30，指定星期支持 1–4 周。星期编号遵循 Swift：星期日=1。跨周周期从 effectiveFrom 起按 7 天计数，不以 ISO 周一重置。civil date 使用 UTC 日历计算，不用持续时长近似跨夏令时日期。

## 分类接口与生产边界

完整说明见 `classification/README.md`。

`classification.New(NameCanonicalizer)` 返回带互斥锁的低层 `Engine`；`Execute(Command, core.TaskGraph)` 使用 expected revision 与 interaction ID 做冲突检查和幂等。支持 create、rename、archive、restore、merge、hardDelete、setCurrent、capture。已有关系保持来源，关系移除保留来源与决策；历史 capture 保存当时名称和颜色，后续改名／合并不改写。

必须注入与 Swift 同版本 ICU NFKC_Casefold 及 whitespace 规则的 adapter。目前只有显式登记测试输入的 `FixtureCanonicalizer`，不将 lowercase 冒充兼容实现。不支持的名字明确失败。

## 集成时的必须条件

1. 同一事务 owner 保管 `recurrence.State` 与 `classification.Snapshot`，按 candidate 模式完成所有变更后，一次持久提交；本批不提供持久事务或共同锁。
2. 父模板分类变更后，调用方依据 `EditableMemberChainIDs`、已有 classification authority 和 `Source` 编排成员更新。`Revise` 返回 `RevisionOutcome.ClassificationAssignments`，只针对新建日期的 chain；复活的 chain 必须保留它自己已生效的分类，不得被父模板覆盖。
3. 交叉校验包含 `State.Validate`、`classification.ValidateReferences`、`State.ValidateCatalogReferences`。仅调用其中一个不足以维护完整快照约束。`integration/transaction_test.go` 提供成功和晚期失败回滚案例。
4. 第一批 `core.TaskKernel` 仍拒绝包含 cycle membership 的输入。不要把第二批任务图直接塞回第一批 kernel 或为规避限制去掉 membership；完整 Engine 的统一命令与事务层仍待移植。
5. 重复 occurrence 必须继续从普通 task pool、calendar 和 generic lifecycle pools 中排除，并在普通 future plans 列表中使用上游的近日期限制。该跨页面过滤与通用任务命令守卫不在本包。
6. `Capture` 是显式低层钩子，不代表今日可撤销 completion 已可冻结。day settlement、return-to-pool、abandonment、copy/change 的历史 capture orchestration 仍必须移植。

## 已知尚未完成

- Swift v7 Codable、canonical JSON、SQLite schema19 与 sync record/ledger 不在本批。Go 默认 JSON 字段不兼容 Swift。`classification.Snapshot` 仅属于此 Go 包的事件重放格式。
- classification prepare/confirm capability、计划 digest、用户确认 token、Zhulong/AI/inherited 权限、undo/redo、自动 presentation approval、catalog reconciliation 尚未实现；相关来源执行明确拒绝。
- 分类 hard-delete 只支持低层未引用实体；完整“解除当前／未来关系并保留历史”的删除编排尚未完成。不得绕过父模板历史引用检查。
- 分类命令全局时间要求不早于前一命令，比上游按实体 mutation clock 更严格；所有时间相等允许，不能无故改变导入时间。
- `strings.TrimSpace` 继承第一批边界，尚未证明与 Foundation 全 Unicode whitespace 完全等价。civil date 约束为有效公历 1–9999 年。
- Swift 跨语言差分 oracle、完整官方 fixture 回放、UI 持久化／重启、并发存储、平台运行验收均未执行。
- Track 边界超过 366 天时显式返回错误；Swift 单条 projection 返回 nil、列表 compactMap 隐藏该条。集成方应明确处理，不能误报历史丢失。

## 运行验证

从本目录，使用已安装 Go：

```sh
GO=../go/bin/go ./scripts/check
GO=../go/bin/go ./scripts/cross-check
```

`check` 只检查 gofmt，不自动改源码；执行 vet 和 race tests。缓存与二进制默认置于 `/tmp/noonmark-domain-extensions-build`。可用 `NOONMARK_DOMAIN_EXTENSIONS_BUILD_ROOT` 指定源码外绝对路径。测试均为合成 fixture。

`cross-check` 为 Linux/macOS/Windows 的 amd64、arm64 生成测试二进制。这是六目标编译，不是六平台运行或桌面验收；只有当前 Linux amd64 实际执行。amd64 不是 32 位 x86；macOS 没有现代 32 位 Go 桌面目标。

实际测试次数、覆盖率、fuzz 与编译结果见 `evidence/verification.json`。`evidence/recurrence-upstream-parity.tsv` 逐条登记上游 51 个重复计划测试的行为映射、部分覆盖和未移植条目；behavior_fixture 表示 Go 合成测试覆盖该行为，不表示已运行原 Swift 测试或完整差分。这些结果不等于完整复刻已完成。
