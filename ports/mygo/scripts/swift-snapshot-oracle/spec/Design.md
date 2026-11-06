# Snapshot representation oracle R3：最小封闭批

2026-11-06，仅设计修订。R1完整原件及hash保存在R1/，R2交付原件及hash保存在R2/，不覆盖历史。正式Go仍 eb924a14d395391c5625e61effddaed500ddb6b9；上游仍43e6eefaae3a04be89cab3e234d470111afd80ef。本次没有实现、编译、测试、产品运行、CI修改或远端写入。遵守已读AGENTS。独审R1的B1–B4已于R2收口；本R3按REVIEW-R2.md修正限内错误raw原件保存与note排列测试范围，不扩大M1；是否可实施仍待独审，不是互通成功。

## 1. 来源和实际serializer（保留R1已认可结论）

Core manifest是14 Swift+Package/LICENSE，共16项；comparison的12pins不是Core数，名称1807不是snapshot证据。固定源和Git blob核验见SOURCE-VERIFICATION.json；保存目录只能证明本地字节吻合，不能替代未来runner commit→rootTree→Core/Storage/App tree认证。新manifest必须合并14 Core（compiled）、ExactDateJSONCoding.swift及NoonmarkDataPackage.swift（compiled selected Storage module）、Package/LICENSE（evidence）、App/NoonmarkStore+NaturalDayPersistenceImport.swift（evidence-only）。这些文件本次已锁；未来实现产生的harness/runner/schema-parser/comparator与workflow必须在候选完成后独审冻结hash。无实现文件现在不得填虚构hash，缺hash时禁止compile。旧名称manifest不能单独认证新工具。

实际链：Store.exportDataPackage(to:)→engine.snapshot→复制preferences.localFirstSyncPolicy.enabled=false→NoonmarkDataPackage.write→encode→validate→currentJSON→ExactDateJSONCoding.encoder→JSONEncoder envelope→JSONSerialization prettyPrinted/sortedKeys/withoutEscapingSlashes。日期是2001 reference epoch Double.bitPattern UInt64（不是Unix或ISO）；decode对应exact UInt64，并验证canonical bytes与shape。write另有atomic写/回读/校验/receipt。本工具不调用Store/write/SQLite；实际serializer capture只用public NoonmarkDataPackage.encode/decode。显式export projection单列，不冒充App导出。

Swift snapshot排序days/date、chains/createdAt-ID、definitions/chainID-sequence、traces/date-continuationSeq-priority-createdAt-ID、subtasks/owner trace chronology-position-createdAt-ID；planned/note原序不能忽略。Go Snapshot/FromDomain保序。合法planned[2,1]与[1,2]不意味着lineage-position改变。不得改产品sort、canonical、prefix/fences以让结果变绿。

## 2. M1缩小范围与不可变输入

M1现只含4个封闭seed、18个必跑case：4 seed-only、3 schedule、11 create正文向量，详SeedRegistry-R2.json。每case使用新engine。seed原始JSON和SHA已冻结在r2-seeds/，全部pending-validation，不能把静态推导称已合法。FieldSchema-R2.json列出M1可出现的全部117个wire字段，对Swift字段/Go字段或Supplement路径、optional、数组、强制空值逐项绑定；不是只列九个sections。未知字段/未支持非空类型在domain decoder前拒绝。

M1禁用returnToPool、complete、undo、addPlanned、任意callback/method字符串、自由seed、调用方checks和identity表。planned排列、多owner已有children由显式boundary seed构造，不能称命令自然可达。M1零history：classifications.formatVersion=1、revision=0，其余12个数组全空；series/ideas/syncEndpointOptions全空。notes只允许create的一个活跃正文，无deleted。已有definitions均sequence=1，statuses均pending。所有非description可选字段均absent。N边界集合固定NoteVectors-R2.json，Go适配值是预先独立假设，Swift若不符就报difference，不按capture改预期。

M2保留R1所有未纳入格，尤其R/X多history、非零audit、旧definition、deleted notes、负priority、精确tie、Date/Now/±0/MaxFinite等，见Matrix.md逐格归属。不得以缩小M1宣称原§8整体执行完成。任何M2格不得计入M1通过，不得删去使报告看似完整。

## 3. 已冻结Go装配（无需taskapp adapter）

每个seed先统一资源preflight，再canonical.Parse(seedBytes, Limits{MaxBytes:262144,MaxDepth:24,MaxNodes:8192})，读取snapshot；v7.ToDomain(snapshot)→(recurrence.State,v7.Supplement,error)，验证Series空、所有空catalog/profile约束；core.NewTaskKernel(state.Tasks)。seed输入是描述JSON，不声称已canonical，故不直接对描述bytes调用Codec.Decode。然后FromDomain(recurrence.State{Tasks:kernel.Snapshot(),Series:[]recurrence.Series{}},固定Supplement)→Codec.Encode，用真实New(Options{Limits:上述限制,ClassificationAuditValidator:nil})。Encode内部Decode必须成功；显式Inspect必须RequiresClassificationAudit=false。每一步用真实结果记录，不能仅Inspect代替Decode。Supplement每次从已核seed快照三section拥有拷贝；seed都使用同一固定preferences/classifications/ideas，没有默认生成器遗漏字段。非零audit到达M1即profile-violation+harness-error，绝不装nil-return fake validator。

create严格调用core.CreatePoolTaskInput{ChainID,DefinitionID,Title,DescriptionText,InitialNote,Now}→CreatePoolTask；正文hex解码通过限额后按NoteVectors预期构造nil或TaskNoteEntry{ID:noteGoID,Body:映射正文,CreatedAt:Now,UpdatedAt:Now,DeletedAt:nil}。schedule用ScheduleFromPoolInput{ChainID,TraceID,DayID,Date,Today,Now,SubtaskIDs:以lineage→seed plannedID确定的map}→ScheduleFromPool。map登记必须每planned恰一项，禁止数组index决定对应。Now=hex bits→core.InstantFromBits；Date是已冻结{year,month,day}，不是wall clock。Kernel.Snapshot重新装State；不复用上一步可变State对象。

Swift seed描述经同一preflight→JSONSerialization构造Foundation canonical fixture bytes（仅输入准备，不是输出oracle）→NoonmarkDataPackage.decode→NoonmarkEngine(snapshot:)；这两个真实验证均须成功。非canonical描述的输入canonicalization单列fixture preparation，绝不能用于cross-decode或洗掉输出差异。先验Swift seed构造可能拒绝，届时invalid-fixture而非跳过。Now用Double(bitPattern)→Date(timeIntervalSinceReferenceDate:)，观察实际结果bits。createPoolTask/scheduleFromPool显式now/today；不调用默认Date。seed-only也capture完整snapshot与实际encode。

Go本身Encode/Decode、Swift本身Decode/Encode及双向cross-decode全部独立执行，经资源preflight后直接给原bytes，绝不预canonicalize/映射/排序。cross Go若RequiresClassificationAudit为true即profile-violation（此M1不应出现）；M2才设计taskapp package-local sealed audit入口，M1没有此adapter需求。

## 4. 身份与比较合同

seed用相同固定UUID，因果key为typed kind+UUID；不得从其数组位置建立key。命令chain/trace由返回值登记；definition由本次唯一新增chain owner+sequence=1；note由本次chain新增唯一note；new day由date；actual由(traceKey, plannedLineageKey)。existing day使用seed身份，未消费的Go DayID不登记成实体。每次差集有缺/多/歧义即mapping-ambiguous；不能靠随机UUID排序或内容相同猜。相同note在chain/trace重复出现按同身份关系核对各处字段。

七个固定comparison layers：entity、array-sequence、raw-package-bytes、raw-export-bytes、swift-self-roundtrip、go-self-roundtrip、cross-decode。entity比较完整FieldSchema所有字段，ID只在此关系视图映射；UTF8精确、stamp UInt64 bits精确、optional absent/value精确，unexpected null属于difference或profile拒绝。array-sequence按schema遍历全部嵌套数组，逐位置替换logical key只是呈现不移动元素。raw层字节直接等值，随机UUID不同亦报告different；实体equal不覆盖raw different。没有一个层可用另一层代替。

每个snapshot点有S.engineProjection/G.kernelProjection（直接字段读取的typed schema view）、S.package/G.package原bytes及S.export/G.export原bytes，后两份只在snapshot复制上关sync。instrumentation projection不是实际serializer，必须注明；它以schema字段读取生成JSON整数bits，不能从package decode反推“kernel原序”。S/G.fromDomain与package parsed structure的边界比对属于entity/array检查，保存Go fromDomain projection；Swift字段projection的空section不可丢失。schema约束错误不得悄悄跳字段。

## 5. 资源、状态、实施闸门

FixtureSchema.md定义所有输入/输出对象、封闭枚举、预算顺序、schema/原件数量；Matrix.md定义有效集合与计数。只有18个必跑case全部valid、所有预定stage实际执行、映射完备、每层有有效观察、全部资源/比较器负例门禁通过，才M1-observation-accepted；different允许作为诚实观察但不是interop-pass。任何invalid-fixture/unsupported/not-run/ambiguous/harness-error导致M1未验收，不用report-complete掩盖。

仅编辑/静态检查的候选实现需另授权；真正Swift capture只能另授权的GitHub-hosted macOS CI，不能在此Linux或production环境运行。14 Core原文件构建，加2 selected Storage原文件独立模块，平台Foundation/CryptoKit，无App/SQLite/runtime。toolchain需固定macOS14 target/Xcode26.2/Swift6及SDK/arch证据。来源fetch阶段是唯一联网阶段；编译及运行阶段均禁网。路径必须新建、外置、拒symlink/重叠，HOME/TMP/cache/build/evidence严格分离；新runner必须有SDK/system/source读allowlist与生产/iCloud/Keychain拒绝见证，不把旧allow-default sandbox视为完整隔离证明。scope布尔值须由实现检查和执行证据支持，不能硬编码。上传仅manifest列出的合成证据，不上传目录。

新的实现/CI授权不在本次。后续表示决策须审所有顶层/嵌套数组、exact binding、prefix/successor、pure fences、导入/UI；不允许代理擅自降完整复刻目标或接Service/v3。

## R3窄范围修订

限内真实serializer bytes先通过单件/件数/累计byte存档预算，使用固定生成名exclusive-create保存完整原件与hash，再判scanner/schema/codec资格；malformed原件仍保存且明确状态，不能当成功观察。超限不保存截断伪原件，仅有界typed诊断。保存不授权进入decoder；任一前置失败的下游domain decoder/clone/bridge计数仍为0。raw不同事实不因另一层失败而删除。V03中note数组排列移M2/not-run，因为M1每owner≤1；M1仍需trace/旧children/planned排列门禁，不能以多note profile拒绝顶替排列差异。18case/32point、4seed和原字段/向量JSON不变，R2命名的冻结JSON继续为本版输入规格。
