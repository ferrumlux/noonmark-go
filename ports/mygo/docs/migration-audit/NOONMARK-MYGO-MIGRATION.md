# Noonmark Go + MyGo 完整复刻：迁移审计与执行门禁

## 结论与证据范围

- 参考仓库：`https://github.com/quboliu/noonmark`；只读基线 commit：`43e6eefaae3a04be89cab3e234d470111afd80ef`。
- 本文基于源码与测试定义的静态审计，不是运行验收结果。没有启动原 App，没有读取任何 production 数据、Keychain 或 iCloud，没有更改原仓库。
- 本次目标是 Go + MyGo 桌面复刻，不是仓库中另一个服务端唯一事实源的 Web/Go 提案。不得把 ADR-0049 的 Web 架构误作当前桌面实现。
- 原作是 local-first 任务领域系统，不能压缩成 `Task{title,done}`。完整功能要求领域语义、数据、同步、AI、安全与原生系统行为都对等，视觉原型仅解决其中一层。
- 十个一级页面、六个设置页、两个独立快速捕获浮窗都必须覆盖。已实现能力与规划端点要分别登记，不能把 S3/WebDAV 或未通过 live 门禁的 CloudKit 声称为现有用户能力。
- 四个关键模块 Core/Storage/Sync/Zhulong 合计 74,942 行 Swift。`Tests` 下有 167 个 Swift 测试/支撑文件，静态找到 1,637 个 `test*` 方法，不代表本次已执行或全部均为默认套件。
- `upstream-test-inventory.tsv/json` 为可追踪迁移清单，初始 `go_status=not_ported`；不应因页面存在而改为通过。

## 1. 必须首先冻结的约束

1. 保留上游 Swift 入口，Go 放独立目录；未达到对等前不 cutover，不覆盖既有数据库。
2. 严格隔离 production/development/e2e/demo/audit/dmg-validation 运行身份。Go 用独立非生产 app ID、数据根与 Key store namespace；测试只用新建合成 fixture。
3. 不自动打开、定位、扫描或迁移原 production 目录。迁移优先用户主动导出的 v7 数据包。未来需要原加密 sidecar 的迁移时另立显式、可审核流程。
4. iCloud live 测试仅在全新 `Noonmark-E2E/AppendOnlyLiveTests/<session-UUID>/<round-UUID>/` exclusive-create；永不删除、覆盖、改名或清理，禁用 mutable head。静态及本地门禁未过不得开始。
5. 先明确目标：建议六个 64 位 target `darwin/amd64`、`darwin/arm64`、`windows/amd64`、`windows/arm64`、`linux/amd64`、`linux/arm64`。若“x86”指 386，另做 Windows/Linux32 位支持决策；macOS14 无32位应用支持，不能给出虚假的完整三平台386承诺。
6. 上游 AGPL-3.0，复用源码与素材应保留 LICENSE、来源与变更说明；分发要求需单独核实，不以自写 Go 语法免除所复制内容的许可证义务。

来源：`AGENTS.md`、`Package.swift`、`LICENSE`。

## 2. 功能清单与真实行为

### 2.1 页面、编辑器与导航

| 面 | 必须复刻的能力 | 主要源码 |
|---|---|---|
| Day Todo | 日期导航、快速输入、分组、置顶与普通优先级、状态动作、子任务、说明与附言、每日复盘、7天趋势和本地信号 | `App/NoonmarkMacApp/DayTodoPage.swift`、`ReviewRail.swift`、`Sources/NoonmarkCore/NoonmarkEngine.swift` |
| 任务池 | 未排期任务、计划子任务、分组/标签、排期、保留历史的移除、统计与有依据的 AI 分析 | `TaskPoolPage.swift`、`PoolDetail.swift`、`NoonmarkEngine.swift` |
| 未来计划 | 普通任务改期、按日负载聚合；重复实例仅展示未来7/15/30天，不能截断普通任务 | `FuturePlansPage.swift`、`FuturePlanDetail.swift`、`RecurringFuturePlanVisibility.swift` |
| 重复计划 | 父计划+独立实例；即将开始/进行中/自然结束/提前停止；向前修订、跳过、停止、完整逐日轨迹 | `RecurringPlansPage.swift`、`TaskCycleTrackView.swift`、`Sources/NoonmarkCore/TaskCycleSeries.swift` |
| 未完成 | 按 chain 去重、未完成次数/延续次数/最近日、展开轨迹、延续、废弃、重新启用 | `UnfinishedPoolPage.swift`、`UnfinishedDetail.swift` |
| 已完成 | 保留完成时刻与跨日路径、部分/完整父子层级、子任务完成记录 | `CompletedPoolPage.swift`、`CompletedRecordDetail.swift` |
| 日历 | 月视图、普通任务状态/摘要、当天跳转、详情栏；重复实例不计入普通日历 | `CalendarPage.swift`、`WorkspaceMonthGrid.swift` |
| 烛龙 | 会话导航、流式内容、scope/recipient披露、结构化产物、Todo diff、确认、应用回执 | `ZhulongConvergedHome.swift`、`ZhulongWorkspaceStore.swift` |
| 飞光 | 独立 note-first 实体、Markdown多行速记、持久草稿、原位编辑、搜索、分类过滤、回看、删除墓碑/恢复领域接口 | `FlylightComposerSurface.swift`、`NoonmarkStore+Ideas.swift`、`Sources/NoonmarkCore/IdeaWorkspace.swift` |
| Sticky Note | 同一飞光的精选投影，不复制正文/创建时间；墙/清单双视图、源修改同步、源删除自动退出 | `Sources/NoonmarkCore/IdeaWorkspace.swift`、`NoonmarkStore+Ideas.swift` |
| 全局任务速记 | App内Cmd+N，进程存活时全局Ctrl+Shift+N；改键失败保留旧组合 | `NoonmarkQuickEntryWindowController.swift`、`CarbonGlobalQuickEntryShortcut.swift` |
| 全局飞光速记 | Ctrl+Shift+I；与页面共享 composer；Enter换行/Cmd+Enter保存/Esc关闭；关闭重启不丢草稿 | `NoonmarkIdeaCaptureWindowController.swift`、`IdeaCaptureSessions.swift` |
| 工作区通用 | 搜索、键盘选择、多选/批量动作、上下文菜单、Undo/Redo、左右栏收放、窗口恢复、日期键盘与滑动 | `WorkspaceShellViews.swift`、`WorkspaceBulkActionBar.swift`、`NoonmarkStore+PresentationUndo.swift`、`WorkspaceSearchIndex.swift` |
| 六设置页 | 偏好、组织、数据、同步、Provider、隐私；双语言/双亮色主题/诗文/快捷键/滑动方向/分类管理/导入导出 | `SettingsPaneContent.swift` |

表中 `App/NoonmarkMacApp/` 与 `Sources/NoonmarkMacRuntime/` 的省略前缀按模块解析；最终实现必须绑定实际文件与 test。

### 2.2 视觉与输入，不可只验一张截图

- 官方 UI 合同：默认1200×768、最小960×720；侧栏220/折叠72，详情栏280、日历详情248；默认侧栏展开、详情折叠；导航行高34，最小交互目标28。
- 应保留冷灰/微暖纸感和中文/English，轻描边、低对比控件、无边框任务行，不以装饰卡片填补信息。
- macOS 流量灯、全尺寸标题栏、关闭窗口保留进程与原生主菜单由平台 adapter处理。Win/Linux采用本地窗口交互，不能绘制不可操作的“仿原生”控制点当完成。
- 中文 IME 是强制验收面。官方有53个输入面契约，覆盖 marked text、候选窗、焦点切换、自动保存、立即退出/重启/故障重试。MyGo 的自绘文本框必须真实验证 composition，不可用 ASCII 键入结果代替。
- 视觉对账应使用用户确认过的真实 App 截图，以及相同一年中段 demo fixture；不以归档 HTML 为 oracle。
- 需测试最小窗口、长滚动、两栏切换不改变窗口外框、高DPI/缩放、中英长文、reduce motion/contrast等辅助设置、重复点击、取消/返回和快捷键焦点。

来源：`Sources/NoonmarkMacUIContract/MacUIDesignContract.swift`；`Tests/Fixtures/tencent-ime-input-surfaces.tsv`；`Sources/NoonmarkMacRuntime/IMETextBindingBuffer.swift`、`InputDraftFlushCoordinator.swift`。

## 3. 领域模型与兼容性契约

### 3.1 模型边界

- `TaskChain`：跨日持续身份及 chain级附言；state仅active/abandoned。不是“今天任务的一行”。
- `TaskDefinition`：当前描述、计划子任务、版本及 supersededAt；变更不得就地改写历史定义。
- `DayTrace`：某自然日事实、definition引用、carryover连接、完成/settled时刻、priority/pin/progress、trace级附言。
- `Day`：自然日身份、锁定时刻、review summary/unfinished reason/tomorrow note。
- `Subtask` 与 `PlannedSubtask`：不同实体；跨日继承 lineage，只携带开放子任务，不能抹除已完成历史。
- `TaskCycleSeries`：父计划、schedule、endCondition、plan revisions、classification revisions、cancellation facts；实例归属不可丢。
- `TaskClassificationState`：分类目录、历史名称、关系、合并/删除墓碑、当前chain分类、历史trace快照/事件、change records。一个主分组+多标签，当前整理不能改写历史分类。
- `IdeaEntry`：Markdown来源库、分类、pin、删除墓碑、创建/更新时间；不是task，Sticky Note不再建第二份数据。
- `AppPreferences`：theme/language有独立writer与clock；本机工作区偏好、密钥、AI sidecar、同步本地ledger不等于领域snapshot。

### 3.2 八种轨迹状态必须完整保留

`pending / completed / unfinished / deferred / changed / returnedToPool / cancelledDraft / abandoned`。

- Day Todo可见：pending、completed、unfinished、deferred、abandoned。
- changed/returnedToPool保留任务轨迹，但不再占一条当日任务行，也不能虚增复盘/日历计数。
- cancelledDraft是内部append-only取消事实，不能进入任一用户历史/AI scope/完成记录投影。Undo不是删掉事实或把时钟倒退。
- 今日未完成可延期，历史unfinished才延续；二者来源与语义不同。避免同chain同时拥有重复active frontier。
- 仅当前日且未锁定的合格trace可以完成/撤销；未来不能完成，历史不能撤销；仍有开放子任务时父任务不能完成。
- settle past days将pending改unfinished并锁日；失败必须原子，不能先改变UI再错误保存。
- 回池要保留轨迹与最新可编辑内容；当前新草稿删除和已有历史任务移除不是同一语义。
- manual progress与weighted subtask progress不同；difficulty权重1/2/3；延续后的进度不得倒退。

来源：`Models.swift`、`NoonmarkEngine.swift`、`TrajectoryTopologyValidator.swift`及`NoonmarkEngineTests.swift`。

### 3.3 重复计划边界

- everyDays间隔1..30；selectedWeekdays每1..4周且非空，锚定series起始日，不是直接用ISO周号替代。
- 结束：durationDays(1..366)、onDate、completionTarget(count/latestDate)；物化窗口有上限，拒绝无界重复。
- 已开始计划修订只影响未来，从明日起；不能重写已发生/当日事实。完成目标在完成日被锁定后才停止后续实例。
- skip仅针对允许的未来实例；stop仅取消未来待办，保留今天延后到未来的事实。
- 普通任务池/历史池/普通日历与重复计划管理投影有明确去重边界，不能把所有实例再次平铺。

来源：`TaskCycleSeries.swift`、`Tests/NoonmarkCoreTests/TaskCycleSeriesTests.swift`。

### 3.4 时间、标识和Unicode是P0，不是序列化小事

1. ID为包装UUID结构（典型 `{rawValue:"UUID"}`），不是所有字段都直接string。Swift非String key字典可编码为交替key/value数组，必须以真实golden确定每种shape。
2. 数据包日期是Swift reference date（2001-01-01 UTC）起秒数的Double **UInt64 bitPattern**。Go用`math.Float64frombits`/`Float64bits`，保留原bit值；不要转`time.Time`后再导出导致精度截断，不可先经float64 JSON数值解析丢失大整数。
3. mutation clock要求严格前进：当墙钟等于或落后现有frontier，取`math.Nextafter(frontier, +Inf)`；拒绝非有限值/无法前进。不能简单`time.Now()`或毫秒整型。
4. `LocalDate`是年月日，不是UTC timestamp。自然日来源是用户当前时区；date-only运算用严格Gregorian UTC noon，避免DST日=24h的错误假设。
5. 分类名规则：空白折叠 + ICU NFKC_Casefold，canonicalKeyVersion带真实Unicode版本。Go `strings.ToLower`或未经版本冻结的NFKC+casefold都不能直接宣称兼容。必须固定规范/测试向量，对ß、全宽、组合字、零宽、土耳其I等对账。
6. 相同时间的稳定排序及UUID大小写、集合去重/排序、缺失字段与null、associated-value enum的Swift Codable shape均进入golden。

来源：`Identifiers.swift`、`ExactDateJSONCoding.swift`、`MutationClock.swift`、`LocalDate.swift`、`GregorianCivilCalendar.swift`、`TaskClassification.swift`。

## 4. 数据与同步边界

### 4.1 数据包v7与SQLite19

- 当前`NoonmarkDataPackage.currentFormatVersion=7`，只接收当前版本，v6等旧版明确拒绝。不要默默“升级”旧包或略去unknown字段。
- snapshot九个字段全部必需：days、taskCycleSeries、chains、definitions、traces、subtasks、preferences、classifications、ideas。
- Foundation canonical规则：prettyPrinted、sortedKeys、withoutEscapingSlashes；读入要求字节本身canonical且整个JSON shape与当前编码一致。不能假设Go `json.MarshalIndent`输出与Foundation相同，冒号/空对象/空数组/Unicode/数字格式均需实际比对。
- 校验重复ID/字段、所有引用、拓扑、clock、分类管理事实、取消witness、非有限日期。解码成功不是完整验证。
- 导出原子写、回读精确相等、再次decode并返回SHA-256/byte count；文件本身不加密，应明确提醒用户保管。
- 导入先preview并确认preview ID、停止同步、检查engine revision未变、exclusive mutation、settle day、事务replace；失败回滚，成功清Undo、同步保持关闭、重新生成当前UI投影。
- schema19同时存展示ISO时间与精确bits；SQLite integer是signed64，不能误存UInt64。必须保留bit重解释语义及约束。
- 建议初期Go拥有独立DB，不直接打开旧Swift DB写入。优先v7桥接；如果承诺数据库二进制/表兼容，增加schema/constraint/ledger/lease逐表门禁，不能仅表名相同。

来源：`Sources/NoonmarkStorage/NoonmarkDataPackage.swift`、`ExactDateJSONCoding.swift`、`SQLiteSchema.swift`、`SQLiteEngineRepository.swift`；`NoonmarkStore+NaturalDayPersistenceImport.swift`。

### 4.2 同步不是复制SQLite文件

- 当前可用路径：显式启用的本地目录及iCloud Drive；S3/WebDAV仅规划；CloudKit adapter存在但不是默认用户路径。
- `SyncRecord`十一类：day、taskCycleSeries、taskChain、taskDefinition、dayTrace、subtask、ideaEntry、appPreferences、classificationBaseline、classificationCommit、traceClassificationEvent。
- canonical record包含record/entity ID、upsert/delete、精确modifiedAt、device ID、payload、reactivationWitnesses。
- classification baseline/commit/trace event是immutable payload。拓扑依赖、去重、terminal证据、classification atomicity、冲突与墓碑不能被粗糙LWW代替。
- v2 replication包含generation/producer/sequence/previous seal chain、content-addressed chunks/root/seal/announcement、durable frontier与publication watermark、fork resolution；重启不能丢pending或把旧事实复活。
- UI必须区分worker状态、converged/waiting/blocked、incident、pending/confirmed/applied/waiting计数，以及lastSynced与lastEffectiveSynced。
- 先移植local-folder并做Swift↔Go混合设备仿真，再做iCloud适配；纯两个Go进程相互收发不是旧数据/旧设备兼容证据。
- macOS的ubiquity container/entitlement/CloudKit不能当纯Go跨平台API。Win/Linux可访问的授权folder seam需要真实验证；没有相同云能力时必须将其列为发布阻断或与用户另议端点范围，不能隐藏功能缺口后称“完整”。

来源：`Sources/NoonmarkSync/SyncRecord.swift`、`CurrentSyncRecordMerger.swift`、`ReplicationEndpoint.swift`、`ContentAddressedFileReplicationEndpoint.swift`；`Sources/NoonmarkStorage/LocalFirstReplication.swift`、`SQLiteReplicationLedger.swift`。

## 5. AI、加密与诊断

- AI Provider种类为OpenAI-compatible/localModel/customHTTP；支持连接测试、timeout/chunk timeout、streaming、structured output。无Provider时普通任务、本地统计与数据操作完整可用。
- 读取scope、远程发送、Todo写入是三层授权；endpoint/recipient/scope实质变化必须重新确认。Provider不得收到DB、内部ID、Keychain值、同步端点。
- Provider输出先artifact与Todo diff，用户确认后走同一领域command，保留receipt。自动分类仅在用户显式开启后作为狭窄例外，耐久job+generation/content/catalog/manual-decision fences不能省略。
- 自动分类job有waitingForConfiguration/ready/running/proposalReady/completed/superseded/cancelled/failed与claim/authorization/checkpoint，必须验证重启/撤销/人工分类胜出/旧响应拒绝。
- 烛龙会话、memory、application journal是独立AES-GCM sidecar，不进普通Todo数据包或同步。只迁移v7数据包不等于迁移所有AI历史。
- Session当前envelope `NOONMARK-ZHULONG-SIDECAR` + version3 + CryptoKit combined GCM；32-byte key，AAD绑定format/session ID。跨语言用固定测试密钥/nonce fixture验证，不接触真实Keychain。
- AI应用横跨engine/session/journal，失败需要通过durable before/after/conflict对账恢复。单SQLite事务无法代替跨sidecar提交协议；未解决pending application时需阻止冲突写入。
- 系统密钥层建议macOS Keychain、Windows受保护凭据/DPAPI、Linux Secret Service，分别验证用户/session/namespace隔离。不可用明文配置文件假装等价。
- 诊断本机allocated bytes≤4MiB、保留≤7天、用户导出≤8MiB；禁止正文、路径、端点、payload、prompt/response、API key、自由文本Error落诊断；隐私哨兵命中阻断发行。Windows/Linux的OS诊断替代与Apple MetricKit分开标示。

来源：`Sources/NoonmarkAI/AIProvider.swift`、`AIScope.swift`；`Sources/NoonmarkZhulong/ZhulongApplicationCommitCoordinator.swift`、`EncryptedFileZhulongSessionRepository.swift`；`Sources/NoonmarkStorage/AutomaticClassificationJob.swift`；`Sources/NoonmarkDiagnostics/`。

## 6. 建议Go模块与并行分工

建议`ports/mygo/`（目录为方案，不是已存在事实）：

- `internal/core`：强类型实体、命令、验证、纯投影、MutationClock；无UI/SQL/网络。
- `internal/daycontext`：时区/休眠唤醒/时钟变化信号，严格date-only运算。
- `internal/compat`：Swift Codable v7/DateBits/Unicode/SyncRecord/envelope，单独golden。
- `internal/storage`：SQLite事务、schema、job、process lease、flush、read-back。
- `internal/sync`：纯合并/ledger/publication + endpoint interfaces；先local-folder后platform adapters。
- `internal/ai`、`internal/zhulong`：Provider/scope与有状态工作区分开。
- `internal/diagnostics`：typed events/privacy/capacity，不允许随意fmt.Errorf业务文本进入日志。
- `internal/platform`：window/menu/hotkey/clipboard/dialog/key store/natural-day/app lifecycle；按GOOS文件隔离。
- `internal/ui`：MyGo控件与presenter；只发command，保存成功才发布新snapshot，不能绕过领域写DB。
- `internal/demosupport`：移植一年故事，通过正式commands生成fixture。
- `testdata/compat` + `cmd/compat-probe`：跨引擎golden与故障回放，真实输出逐条对账。

并行边界：A领域/日期/分类；Bcompat/storage；C UI/presentation；D sync；E AI/sidecar；F platform/release。A/B先输出稳定command/schema契约，C可使用合成snapshot但不得创建第二套业务状态机。

## 7. 阶段与退出条件

### P0：先冻结基线与证明技术可行性

1. 锁定上游commit、许可证、六target定义与MyGo版本；完成屏幕/操作/实体/test映射。
2. 在隔离Mac上生成官方一年demo数据包、snapshot投影、Swift Codable边缘golden与sync/sidecar样本；写manifest含source commit、生成命令、hash、typed预期。不能手写“Swift样本”冒充oracle。
3. 在六target实机或原生CI跑MyGo窗口、中文IME、剪贴板、文件选择、快捷键、key store smoke。支持平台的编译与运行分别记录。
4. 先实现DateBits/ID/enum/dictionary/canonical JSON/Unicode/LocalDate兼容层，往返测试。

退出：所有wire golden按预期接受/拒绝，MyGo基础输入及渲染已在目标平台有真实证据；未具备目标runner的状态只能是未验证。

### P1：完整离线产品内核

1. 逐组移植Core、classification、repeat、idea、undo、natural day并绑定上游test。
2. 建立SQLite仓储与串行mutation lane、写失败不发布、导入/导出/回读、崩溃恢复、输入草稿flush。
3. 把全部页面与设置接入同一引擎，移植一年demo；完成中英/主题/快捷键/搜索/多选/编辑器。

退出：无网络无Provider完整使用；重复计划、历史事实、子任务层级、飞光、复盘正确；Swift→Go→Swift的v7数据可对账；崩溃重启不丢已确认输入。此时仍不是完整版本，因为sync/AI未完成。

### P2：同步与AI能力对等

1. 纯sync merger→SQLite ledger→local folder→Swift/Go互通→平台云端adapter，按依赖逐层推进。
2. Provider mock/contract→stream/cancel→artifact/diff/permission→AES-GCM sidecar/journal恢复→自动分类fences。
3. 显式live Provider与真实云端在安全门禁通过后单独运行，不把mock结果称live。

退出：冲突/断网/乱序/重复/重启/损坏/producer fork等对账；AI未授权不读取/不发送/不应用，失败没有假成功；sidecar迁移范围明确。

### P3：六平台发行门禁与切换

1. 每target原生构建、启动、业务E2E、IME、性能、崩溃/磁盘故障、辅助功能、安装/卸载与升级验证；目标处理器执行或有明确真机报告。
2. macOS签名/entitlements/DMG、Windows安装与签名、Linux发行依赖和桌面集成各自验证。cross compile成功不算运行成功。
3. 独立Go profile试用与用户主动导入，保留原Swift App/数据不动。导入预览、摘要计数、历史快照与失败回滚都可验证。
4. 对等矩阵没有未解决关键项、原始用户路径门禁全绿，才讨论默认切换。回滚是回到未动的原版与已验证导出包，不对生产做破坏性逆向schema迁移。

## 8. 必须落地的兼容golden与测试

### 8.1 Golden corpus

- `v7/empty`、`v7/one-year-demo-zh/en`、`v7/nested-submillisecond-dates`、`v7/classification-merge-delete-history`、`v7/recurring-full-track`、`v7/cancelled-draft-witness`、`v7/idea-pin-tombstone`。
- negative：v6、unsupported/missing formatVersion、missing required field、unknown/nested extra field、重复key、非canonical空白/顺序、断引用/重复ID、NaN/Inf bit patterns、错误历史/时钟、无witness的cancelledDraft。
- UTF/编码：UUID casing、非String key字典、Swift associated-value enum、sets顺序、empty objects/arrays、null/omission、特殊引号/slash/反斜线/非BMP字符、UInt64大于2^53/Int64.max。
- sync：每种record、canonical digest、immutable identity collision、classification commit跨记录原子边界、reactivation witnesses、out-of-order/dedup、delete-vs-upsert、terminal clock、fork及watermark丢失。
- crypto：固定合成session/memory/journal，正确key与AAD、错key、错session ID、截断、翻bit、unknown version、重放/并发冲突；不存真实用户key。

每个golden需要`input`、`operation(s)`、`expected outcome/error`、`expected snapshot/projection`、`generator commit`。数据包canonical校验测严格字节；Set/非String字典语义对账使用明确归一化，不误把合法集合顺序差异当领域差异。

### 8.2 双引擎差分

- 一份command序列依次运行Swift与Go，每步比对完整snapshot与DayTodo/pool/future/unfinished/completed/calendar/ideas投影；比较拒绝类别和“拒绝后状态未改变”。
- 注入固定时钟与ID源；无法在原引擎注入全部UUID时，以先生成的fixture ID或严格双射匹配比较，不能随意剔除引用/历史字段。
- 复用确定性simulation基本seed与12操作，并扩展repeat/classification/idea/sync；记录seed/step供复现。
- 对幂等、交换、结合和收敛性质仅在原合并算法允许的范围设断言，不能凭“应当CRDT”假设强行改语义。

### 8.3 最小强制回归组

1. Pool→today→subtasks→partial progress→complete→undo→day rollover→历史锁定。
2. today defer→future edit→withdraw/return pool/reschedule，源历史及计数不虚增。
3. unfinished continuation→completed，未完成池消失，完成轨迹和lineage保留。
4. changed/returnedToPool/cancelledDraft投影，内部取消事实永不泄漏。
5. recurring创建/转入/修订/skip/stop/completion target；历史不变、今天边界、最后实例延后。
6. 分组rename/archive/merge/delete；历史名称/快照正确，人工整理胜过过期AI。
7. 飞光保存/关闭/重启/编辑/删除/恢复/Sticky投影、搜索与回看互斥集合。
8. v7双向往返、导入cancel/expired preview/engine changed/sync running/SQL失败，所有失败无半套新数据。
9. 两设备同改/乱序/重复/断网/重启/损坏/终态/取消/producer fork；禁止旧事实复活。
10. AI流取消/timeout/恶意结构/新授权scope/换endpoint/重复apply/engine已保存session失败/journal清理失败。
11. IME53输入面、立即退出、草稿恢复与故障重试；并发多窗口/多进程数据根lease。
12. 诊断容量/隐私；故障→锁等候→拒绝写→异常退出→重启→用户导出证据闭环。

### 8.4 可复用上游测试入口

- `Tests/NoonmarkCoreTests/NoonmarkEngineTests.swift`、`TaskCycleSeriesTests.swift`、`TaskClassificationModuleTests.swift`、`IdeaEntryTests.swift`、`SnapshotUndo*`、`MutationClockBoundaryTests.swift`。
- `Tests/NoonmarkStorageTests/DataPackageTests.swift`、`SQLiteSchemaTests.swift`、`SQLiteStoreRuntimeTests.swift`、`LocalFirstReplicationTests.swift`、`ReplicationV2BackupDomainStressTests.swift`。
- `Tests/NoonmarkSyncTests/*`：canonicalization、immutable collision、atomicity、definition/subtask topology、undo sync。
- `Tests/NoonmarkZhulongTests/*`、`NoonmarkAITests/*`、`NoonmarkZhulongAITests/*`：scope、permission、provider、journal/crypto。
- `Tests/NoonmarkMacRuntimeTests/*`：natural day、IME/draft、navigation、search、selection、shortcut。
- `Tests/NoonmarkDiagnosticsTests/*`与`Tests/NoonmarkSimulationTests/NoonmarkDeterministicSimulationTests.swift`。

## 9. 交付状态格式

每feature记录：上游source/test → Go实现 → 单元/差分/故障测试 → 六target运行证据 → blocker。状态只能用未移植/实现中/已实现未验证/已验证/阻断，不能把截图、build、mock、静态合同混成“完成”。

本次交付为审计和迁移计划，未执行Swift或Go业务测试；实际移植、原生六平台运行、真实Provider和iCloud互通均待实施。
