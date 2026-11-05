# Noonmark v7 候选数据包 codec

第三批独立 Go 模块：`github.com/ferrumlux/noonmark/ports/mygo/domain-codec`。依据 `quboliu/noonmark@43e6eefaae3a04be89cab3e234d470111afd80ef`，AGPL-3.0-only。

**当前是隔离的候选实现，不是已验证的 Swift 互通层，也不是生产导入入口。** 没有 Swift/Foundation 运行时 oracle 或完整官方 fixture 回放。非空分类审计缺少完整 validator 时，`Decode` 和 `Encode` 明确拒绝；不会悄悄降级成第二批 catalog。

## 已实现

- 九个 snapshot section 全部保留：days、taskCycleSeries、chains、definitions、traces、subtasks、preferences、classifications、ideas。
- 根据上游 Codable 声明校验嵌套字段、必需值、optional 的省略规则、枚举 payload 和所有已声明分类管理／审计结构。未知字段及显式 null 均不被静默丢弃。
- 区分 `{ "rawValue": UUID }` 的强类型 ID 与 UUID scalar。接受合法大小写 UUID；原 wire 字节保留拼写，转换为前两批领域 ID 时规范为大写，字典唯一性按 UUID 身份检查。
- 精确保留 Swift 日期：JSON UInt64 整数承载 `timeIntervalSinceReferenceDate` Double bitPattern，不经过 float64 JSON 解码，不改成 Unix 毫秒。拒绝 NaN/Infinity 位模式，保留 sub-millisecond 与负零位模式。
- Swift 非字符串键字典使用交错 key/value 数组。校验偶数长度、key 类型、大小写等价重复身份及 key/value identity 一致性。
- 重复 JSON key、非法 UTF-8、未配对 surrogate、尾随内容、深度/大小/节点超限和人工 AST 循环引用均明确失败。
- 复用前两批任务及重复计划不变量，增加偏好 writer、飞光/tombstone/pin、分类当前/历史引用、merge 图和 tombstone 常见校验。
- `WriteVerified` 只有在完整写入、readback 字节相同、再次成功解码后才返回 byteCount / SHA256。它操作调用方提供的隔离流，不选择目录、不替换数据库、不声称完成原子安装。

## API

- `canonical.Parse` / `Marshal` / `ValidateCanonical` / `Shape`：有界、数字无损的 JSON AST，Foundation 风格整数格式候选，及上游数组 shape multiset 算法。详细边界见 `canonical/README.md`。
- `v7.New(Options)`：创建 codec；默认解析上限 16 MiB、128 层、1,000,000 节点。限制可收紧；候选 writer 仍使用自己的默认硬边界。
- `Codec.Inspect(bytes)`：验证候选字节、schema、任务/重复图和公共引用，返回独立复制的 `Inspection`。`RequiresClassificationAudit` 不代表已通过审计，也不是允许导入的凭证。
- `Codec.Decode(bytes)`：在 Inspect 之后执行审计门禁，返回字段私有的 `Document`；`Document.Snapshot()` 总是深复制。
- `Codec.Encode(snapshot)`：重新完成所有校验，生成候选规范字节，不缓存之前的通过结果。
- `Codec.Read(io.Reader)`：限量读取，拒绝溢出的 reader 配置。
- `Codec.WriteVerified(snapshot, writer, readBack)`：隔离暂存流与 readback 校验。调用方必须自行完成事务、fsync 和平台原子安装。
- `v7.ToDomain(snapshot)`：返回 `recurrence.State` 与 `Supplement`，后者完整保留 preferences、Swift classifications 和 ideas。
- `v7.FromDomain(state, supplement)`：构建全部九个 section。不能省略 supplement，也不能用第二批 `classification.Snapshot` 代替 Swift 分类审计。

`Options.ClassificationAuditValidator` 接收**整个 snapshot 的复制**，必须是无副作用、确定性的完整验证器。它需要实现 ICU 版本与名称完整性、分类 revision/event 计数、canonical audit 排序、来源/决策授权事实、change record/receipt/plan digest、管理历史与全部交叉引用。当前没有生产实现。测试中使用的精确合成 fixture allowlist 只验证 codec 接口，不代表合法的上游审计。

空分类必须同时满足 formatVersion=1、revision=0 且全部集合为空，才不需要外部审计验证器。

## 与前两批集成

相邻放置 `domain-core`、`domain-extensions`、`domain-codec`；本模块通过 go.mod 的本地 replace 引用前两批。没有修改这些依赖模块、Swift 参考仓库或 UI prototype。

推荐顺序：隔离暂存输入 → `Inspect`/`Decode` → `ToDomain` → 统一事务 owner 完整检查 → 明确授权的安装。应用不能将 `Inspect` 当成导入完成，也不能为了通过校验丢弃分类、飞光或偏好字段。

所有 codec、fixture 和测试都只处理合成内存数据，没有读取、定位或操作 production、iCloud、Keychain 或个人资料。

## 测试与证据

```sh
GO=../go/bin/go bash scripts/check
GO=../go/bin/go bash scripts/cross-check
```

缓存与编译产物默认在 `/tmp/noonmark-codec-build`，可通过 `NOONMARK_CODEC_BUILD_ROOT` 指向源码外绝对路径。`check` 只检查格式，执行 vet、race tests；`cross-check` 编译 macOS/Windows/Linux × amd64/arm64 的两个测试包，**不等于六平台运行或桌面验收**。

`v7/testdata/synthetic-v7-candidate.json` 是本 Go 候选生成、固定下来用于防漂移的合成 golden，不来自 Swift，不是导出的个人数据。测试覆盖任务/重复计划历史、精确时间、字段形状、UUID、字典、审计门禁、回读校验和恶意输入。验证结果及完整日志位于 `evidence/`。

## Swift oracle 后续门禁

`oracle/FoundationCanonicalOracle.swift` 只读取固定合成 fixture，通过 Foundation JSONSerialization 的 `.prettyPrinted/.sortedKeys/.withoutEscapingSlashes` 输出。它不会导入 App 或访问运行资料。

```sh
SWIFTC=/path/to/swiftc bash scripts/check-foundation-candidate
```

该检查即使 MATCH，也只证明这个 fixture 的 Foundation 格式候选相同，不能证明 NoonmarkDataPackage.decode 接受、领域语义一致或全部 Unicode/date 边界通过。当前未执行。

完整互通验收还必须在隔离 Swift 构建环境中：
1. 用原 `NoonmarkDataPackage.encode` 生成空图、全字段/全状态、分类合并/删除/审计、飞光与重复计划 fixture。
2. Go decode → encode → Swift decode，并比较 typed snapshot；反向同样执行。
3. 单独比对空容器、Unicode/控制字符、slash、UInt64 边界、负零、sub-millisecond、UUID 大小写、字典/Set 顺序和所有未知字段拒绝。
4. 对当前 31 个 `DataPackageTests` 建立真实差分，不把本模块形状覆盖或合成 round-trip 计为 Swift 通过。

## 已知保守差异

- Formatter 目前是整数限定候选；空容器布局、Unicode escaping/collation、Foundation 特殊行为尚无真实 oracle。
- 为限制不可信 revision ancestry 校验成本，每个重复父最多 2,048 个 plan/classification revision，整个包最多 4,096 个；超限明确拒绝，不截断。分类 merge 图使用已解析路径缓存，避免重复长链遍历。
- 默认只支持有效公历 1–9999 年及前两批严格领域约束。字符串 trim 与 Foundation 全 Unicode whitespace 等价性尚未证明。
- Go 的完整分类审计 validator、ICU adapter 和官方 integrity digest 尚未移植。
- 没有 SQLite schema19、增量同步、加密 sidecar、文件原子安装或生产迁移流程。
