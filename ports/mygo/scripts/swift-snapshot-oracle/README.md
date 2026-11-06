# 独立 Swift / Go 快照表示观察器（R3 M1）

## 当前 I1 source-rebind v2 局部候选

冻结 spec / Swift 原件及 eb924 身份保持不变；Go 实现来源独立固定为 0a680da10c4119d48ca6176e9efcbf8b6584dcef / tree 9ea10c1a215b3ab907517cda445b1dc9062d2412，不借用旧3346或R3结果。下方R3说明保留为历史合同，不能理解为新Go源码与eb924逐字相同。

本阶段仅实现来源原件认证、139路径/133独立OID清单、完整255最大角色清单、固定205-byte stdin与50352-byte五件framed stdout的解析/监督/机械收据及纯离线复算、全部路径原件和额外batch副本的留存计数。两份零字节文件各保留独立空原件，不算内容chunk；196个非空chunks及208上限均不变。transportError成功的唯一表示为字符串空串，null及其它类型拒绝。

最终runner commit/tree与所有255角色的实际argv/cwd/env/phase尚未绑定。main和生产batch入口在任何子进程前明确拒绝启动。内部低层监督器只供本地合同，完整source acquisition/export、binding4/provenance4、pending生命周期接通、两Darwin闭包、公共CA、macOS sandbox和HTTPS仍未完成。本地收据明确scope=local-source-batch-mechanical-only、runtimeAdmission=false；离线核对原件与见证一致性，不重现OS执行，也不升级完整I1/Swift对等结论。


本工具不是产品 runtime、迁移器或发布步骤。只接受冻结的4 seed、18 case、32观察点、11正文向量、117字段及19上游来源文件。所有 history / R / X / M2 都不运行。主产品、既有 name oracle 和其 pins 不变。

## 运行与真实性

Python 合同及 Go probe 可在外置 BUILD 执行；真正 Swift 只允许经审查后的 GitHub-hosted macOS runner。当前候选在 Linux 上**没有编译或运行 Swift**。Python 合同、Go 实际32点及合成协议测试不能证明 Swift 互通。

- 本地合同：`PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -p 'test_*.py'`。
- Go：从 `ports/mygo`，按 `go-probe/README.md` 使用官方 Go1.27.3、外置 GOCACHE/BUILD、已核 module cache。
- Hosted：仅 `.github/workflows/swift-snapshot-oracle.yml`。只读 contents 权限、无 secret、无发布；精确 paths / 手动触发。候选须先独审并由父任务决定是否合并与触发，本实现不远端写入。

runner 固定 Xcode26.2 / Swift6 / macOS14 target，独立 source、build、HOME、TMP、cache、run。只在源码 fetch 阶段联网；编译及运行 deny-default sandbox 禁网。production / iCloud / Keychain 禁止见证使用虚拟哨兵，不探测真实资料。原14 Core + ExactDateJSONCoding + NoonmarkDataPackage 不改写；Package / LICENSE / App 导出调用源只作证据。Go基线 eb924a14 的产品源码须原样。

## 流程

1. 固定 SHA / commit→tree→blob 认证；source/tool manifest 的改hash、缺失、额外、重复项负例在首个编译调用前执行。
2. 所有入口先限长、token深度/节点/文本/数组/member预算，再精确 schema / 冻结值；预测完整增长及九份输出总预算，之后才 domain / clone / bridge / command / serializer。
3. Swift 使用真实 NoonmarkDataPackage encode/decode，日期保留 reference-date Double 的 UInt64 bits。独立直接字段投影不是 serializer；classifications.formatVersion 是上游私有编码常量1，投影显式注明此常量，真实package另外比较。Go真实 ToDomain→TaskKernel→FromDomain→Codec，无 taskapp adapter / fake audit validator。
4. 原始 projection/package/export/self 字节先完整 exclusive-create 保存与hash，再作输出资格检查；限内错误raw保存malformed-raw，超限只留有界诊断，不产生截断原件。export只在独立快照关闭sync。
5. typed因果身份映射只用于字段/关系视图；数组顺序独立观察，原始bytes不映射、不排序。实际双向cross直接交给接收端原bytes；Go Inspect拒绝时显式Decode保持未运行，记录独立终止阶段。
6. 三轮实际18case观察；只有首轮32点计分。V01–V09执行器含来源、身份/顺序/字段、raw突变真实cross、统计缺项、各入口资源、进程超限、两端24固定恶意mutation及正控制、三轮真实性、隔离负例。负例永不加入业务分母。

controller 重新核对原件/hash/schema并计算 comparisons 与 summary，不信子进程自报通过。`reportComplete`、`observationAccepted`、`interopAccepted` 分开；任何invalid-fixture、unsupported、缺证据、未运行或映射歧义都不能补齐32点。实际different可有效观察，却不能算interop equal。合同绿不能盖住观察差异；真实门禁失败保留fail。

## 文件与证据

- `spec/` 原样保存已独审R3资料及继续有效的R2命名冻结输入。R1/R2历史归档不作为运行规范。
- `bounded.py` / `process.py`：独立输入、输出、原件、pipe预算。
- `controller.py` / `comparator.py`：固定调度、因果身份、七层比较及可复算统计。
- `gates.py` / `gates_comparison.py`：实际负例执行器；synthetic unit tests明确非runtime。
- Swift六文件、Go probe：合成状态真实操作与实际codec。
- `runner.py`：来源认证、只读隔离构建、哨兵、明确文件清单上传。
- `tool-manifest.json`：候选全部工具/规格/测试/workflow的固定hash；不包含自身，manifest本身须由独审记录hash冻结。缺失/不符禁止compile。

所有产物/日志/RED证据放外置 snapshot-oracle-build；repo内不建立缓存或binary。上传仅已核manifest列出的合成原件和报告，不上传目录、生产数据、token或账户资料。

## 许可

新观察器源码 SPDX AGPL-3.0-only；保留根目录及 ports/mygo 的 AGPL-3.0 LICENSE 和既有来源声明；MyGo 等依赖继续分别遵守其原许可。上游源按19项manifest原字节获取，LICENSE亦固定保存，不删除或重写notice。工具是独立研发参考，Go+MyGo产品无Swift运行依赖。

## R2 实现修订（待独审）

- 每次真实执行先保存原始 stdout、stderr尾段、结果、解码字节和实际退出码，再核对协议。进程信号、异常退出、旧目录、缺少本次执行见证都不能产生schemaValidWitness或有效点。
- 正常双向cross保留逐方向比较及原始结果/decoded引用。`offline.py`只从这些有hash的原件重算三轮比较、身份映射、点数与门禁，不用子进程或summary自报的通过值。
- shared-entry矩阵实际调用普通入口，并记录真实预检、materialize及域操作事件；正控制、防护旁路和后移故障也必须被检测。注入的资格检查bytes有独立hash，不冒称原始已归档malformed输出。
- CI只有重新验证的32有效点和V01–V09全通过才exit0；真实差异可以observationAccepted=true而interopAccepted=false。失败也按封闭名单打包有界诊断，workflow用always上传已生成bundle。
- 原始收据、逐方向字节、来源/工具manifest和固定构建命令收据通过分段hash清单收入单个tar；没有递归目录上传。合同与故障注入材料始终不是Swift互通证据。

## 第三轮证据修订（待独审）

完整强制清单固定789个子项，逐项ID、kind、所属V、来源与正负控制嵌入gate_receipts.py；生成清单本身不是执行见证。V01–V09在线与离线采用同一完整集合。archive实际前后ledger/写入与qualify事件、资源进程实际deadline/读数/终止、平台原始facts及命令均须有本次绑定，缺项、错kind、陈旧或saved汇总不一致全部失败。offline只读原件、计算哈希和确定性比较，不重新执行decoder、Python资源进程或平台probe。

run-binding固定本次runID、工具manifest、来源manifest和catalog哈希。全部child以固定分片索引保存；process原件索引也按128项分片，避免完整364次执行加192方向的合法metadata超过单件字符串预算，不提高原资源限制。

固定19来源git cat-file仅使用实际观测bytes/SHA/Git blob的source-object收据，不将合法大源码复制进普通128KiB stdout；源提取原4MiB上限与普通命令上限不变。超限/坏路径/非regular原件只产生有界排除诊断，不截断伪raw，不删除原件；任何排除强制失败，但仍尝试可验证失败包。1MiB诊断保留额包含在总80MiB内，正常包和紧急包都核算已承诺原件、metadata及tar成本，紧急包不再次读取poison。

完整本地解包合同必须带source-gates、绑定、所有child、原始资料与清单，不再从另一目录补件。合成协议合同只能证明控制流程与证据链；Swift270、真正跨语言32点及macOS隔离仍须后续授权hosted实证。

### I1 局部 source capture（不授予运行准入）

内部 `_i1_capture_source_stage` 只从已认证的固定 loose bare DB 运行134条逐路径
Git读取及唯一五件batch；不接受调用者给出的产品bytes作为成功读取见证。DB准备
不计入135条执行收据。当前DB精确2commit、36tree、133blob，固定HEAD/config与
空refs，不支持pack、alternates、promisor、链接或额外条目。

来源仍为policy冻结的productSourceBase，不是本次runner HEAD；runnerCommit保持
null，`i1_require_source_execution_bound` 与 `i1_run_source_batch` 仍fail closed。
本切片不实施execution descriptor、binding4/provenance4或其余120角色。

每条调用前后重读DB精确全集及三根no-follow身份，单件stdin为DEVNULL，stdout
专用4MiB；ordinary 128KiB、Swift19及batch205/50352合同不变。134条原件按139
路径索引保留，共享OID不合并，两份零字节原件及五份batch子原件均保留。

sourceExpected由认证原件构造；postWrite与postExport分别实际重新打开、枚举并
读取全部146文件，比较内容、mode及两次dev/inode。导出0444/0555与Git mode分列。
当前7probe 63157bytes，146输入9525260bytes，139产品9462103bytes/196chunks。
原历史62811/9524914不是本次输入身份；变更工具后须重新验证45路径manifest。

`_i1_verify_source_capture(members, trusted_manifest_sha)` 只消费显式归档成员bytes，
固定重建原件、收据、checkout/DB/导出测量与引用闭包，不读取本机源码。
`_i1_verify_source_capture_archive(archive_bytes, trusted_manifest_sha)` 另核完整
规范USTAR字节、固定元数据及精确预算；可信manifest摘要必须来自已核执行身份，
不能从未信任包自行取出后声称已认证。源raw保留仍限20MiB/8192，单成员128KiB，
完整普通预算79MiB，诊断1MiB。419次固定阶段/写入用量采样与封包后用量记录是
实际观察值，不是内核配额。失败保留partial原件/收据、诊断及导出，不重试或删重来。

即使snapshotReady=true，runtimeAdmission仍false。离线只能复核记录与原件一致，
不能重演历史OS退出/磁盘行为；no-follow前后测量不能排除恶意同用户在两次测量
之间替换后复原，readonly不是sandbox。本批没有编译、真实Swift/macOS、CA、
HTTPS、完整I1准入或产品对等结果。
