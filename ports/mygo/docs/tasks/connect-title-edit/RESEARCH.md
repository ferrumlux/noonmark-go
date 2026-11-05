# 标题编辑复刻研究

基线：正式移植仓库 HEAD d85021f；只读静态研究，没有修改仓库、提交、网络 AI、生产资料或实际 App 操作。上游目录 noonmark-reference，下文 Swift 路径均相对该目录。Go 路径相对 noonmark-go/ports/mygo。独立 domain-core/core 也已阅读，但实施必须用正式仓库的 domain-core/core。

## 必须先解决的风险

1. 不能直接解除历史 definition 冻结。Swift 在版本分叉时会修改旧 definition 的 contentUpdatedAt、supersededAt、supersededByDefinitionID，但 title、descriptionText、plannedSubtasks、ID、chainID、sequence、createdAt 不变。CaptureEmptyHistory 当前以完整 canonical 字节比较旧锁定 definition，合法分叉也会拒绝。这是缺失命令的保护边界，不是该删掉的保护。
2. 核心清空与 UI 清空不是同一语义。saveTaskTitleInput / renameTaskTitle 允许已有标题为空，创建不允许；但当前详情编辑器 .nonemptyTrimmed 明确不提交空白，结束编辑或 lifecycle flush 恢复 lastAcknowledgedText。不能以核心单测为由把 UI 清空实现成永久无标题。
3. 返回池状态虽然 Swift 可编辑，但当前 ValidateEmptyHistory 仅允许 pending/completed/unfinished，returnedToPool 仍不在运行时子集。完整核心命令可以研究/实现该状态，当前 service 不应顺便放开更多状态或分类 supplement。
4. 当前没有 Swift 可执行工具链（command -v swift 无结果）。所有 Swift 结论均为源码与官方测试断言，未运行真实 Swift oracle，不能宣称跨引擎或真实 Mac UI 已验收。

## 上游精确命令语义

来源：Sources/NoonmarkCore/NoonmarkEngine.swift:1216–1322、3556–3568；Tests/NoonmarkCoreTests/NoonmarkEngineTests.swift:74–184、188–250。

两个入口唯一区别是 reusingEditableDefinition。顺序为：

1. 标题 CRLF、CR 转 LF，再 Foundation whitespacesAndNewlines trim；允许结果为空。内部换行保留。
2. 找 chain；必须 active，否则 chainAbandoned。
3. chainTraces 只收 formsDayHistory，排除 cancelledDraft。
4. chainTraces 为空可编辑；不为空则至少存在 pending 或 returnedToPool，否则 invalidTransition。不是简单“只要 active 就能改”。
5. 任何 pending 日期早于 today 均 immutableHistory；所有 pending 日必须 unlocked。上述检查早于相同标题的 no-op。
6. 取 current definition；标题相同则 return，所有时钟和实体均不动。
7. currentDefinitionHasHistoricalReference = 任一 chainTrace 指向当前 definition 且 status != pending，未要求该日 locked。即 returnedToPool 或尚未锁日的 completed 也能阻止复用。
8. chainTraces 为空：两入口均原位修改当前 definition 标题和 contentUpdatedAt。
9. saveTaskTitleInput 且无上述历史引用：原位修改标题和 contentUpdatedAt，即使有 pending 也不生成新版本。
10. 其余情况：新 definition sequence = 同 chain 全部 definition 最大 sequence + 1，复制 descriptionText/plannedSubtasks，title 为新值，createdAt/contentUpdatedAt=now，superseded 字段为空；旧 current definition 的 contentUpdatedAt 前进，supersededAt=now，supersededByDefinitionID=新 ID；所有 formsDayHistory 的 pending trace 重绑新 definition 并推进 contentUpdatedAt，其他 trace 不变。不是产生 changed trace。
11. 最后 touchChain(now)。没有文本修改以外的 notes、分类、subtasks 或 day lock 变化。

注意：显式 renameTaskTitle 只要 chainTraces 非空就分叉，即使全是 pending；每次不同标题都可能新建版本。输入 autosave 应用 saveTaskTitleInput，不可每个字调用 rename。

## 建议可实施 API（尚未实现）

核心新增强类型输入：

    type EditTaskTitleInput struct {
        ChainID TaskChainID
        Title string
        Today LocalDate
        Now Instant
        NewDefinitionID TaskDefinitionID
    }
    func (*TaskKernel) RenameTaskTitle(EditTaskTitleInput) error
    func (*TaskKernel) SaveTaskTitleInput(EditTaskTitleInput) error

实现共享私有 updateTaskTitle(in, reuseEditable bool)，沿用 mutate 深拷贝、ValidateTaskGraph、成功后发布。NewDefinitionID 仅在分叉路径需要有效且未占用；复用/no-op 不消费身份，不生成未使用实体。service 可预分配随机 ID，但核心不能隐式造身份。Now、Today、sequence 溢出与 UUID 冲突必须 fail-closed；严格时钟沿用现有核心工具，不以 time.Now 绕开注入时间。

service 暴露按 chain 的 SaveTaskTitleInput 与 RenameTaskTitle；UI trace 入口额外核验该 trace 自身当前仍可编辑，避免失效的历史详情保存回调误改同链新任务。这是 UI 路由保护，不要伪称 Swift engine 有 trace 级参数。两个入口都锁定 service、以 s.today 校验，并在完整 snapshot 候选上一次 audit/encode/commit 成功后同时发布 graph/snapshot。磁盘失败、冲突、关闭、日期边界拒绝均不可留下内存标题或新 definition。

不建议给 CaptureEmptyHistory 增加通用 allowHistoricalMetadata bool。保留现有默认严格入口；新增命令专用 capture/audit 路径，只接受已审核标题变更证据，并重新核验 before/after 的确只包含这一命令允许的差分。证据不能是任意调用者可伪造的“允许忽略字段名单”。可以用同一命令对独立 before 副本重放，验证 after 精确匹配，然后调用共享严格审计器的单一受限 supersession 分支。

## 历史与事务不变量

- prior 必须仍严格匹配 before 五大 task section；完整 supplement 不得丢弃。normalizeSnapshot 仍走 encode/decode/v7.ToDomain，不可跳过。
- 已锁 day ID/lockedAt、不论已有还是新加的已锁日 trace、关联 subtasks，全部维持原冻结规则。
- 旧历史 definition 的内容投影必须逐字段/完整 canonical 一致：id、chainID、sequence、title、descriptionText、plannedSubtasks（含所有 ID、lineage、position、clock）、createdAt。
- 唯一容许旧历史 definition 差分是合法当前版本的一次 forward supersession：旧 SupersededAt 原为空；新后继必须本次新增、同链、唯一 current、sequence=max+1，SupersededAt=Now、ContentUpdatedAt=合法 Now，后继指针精确命中。旧版本已经 superseded 后，其这三个字段也不得再改。
- SupersededBy 在未 superseded 的旧对象上可能是 restoration witness，不能笼统要求原指针必为空；core/validation.go:188 注释与 core_test.go:243 已保留这一语义。对有 witness 的编辑需要官方 fixture补证，不能随意删除支持。
- 新 definition 除标题、身份、sequence、初始 clock 外，内容必须复制旧 current。pending trace 仅 definitionID/contentUpdatedAt 可变；历史 trace 指向旧 ID。
- 标题变更本身不能增加、删除或重写 classification snapshots/events/revision。若合并自然日结算，必须分清结算产生事件与标题变更本身；首批最好不扩展结算组合。
- 不因为这次标题能力开放 rich catalog、recurrence、ideas、returnedToPool runtime 或任意 metadata 修改。
- 泛化结构验证无法证明“历史从未被改”。读取合法 snapshot 本身并无 before 参照，不应称 normalizeSnapshot 可以替代命令级差分审计。

## UI 准确行为

来源：App/NoonmarkMacApp/TaskDetail.swift:25、37–47；DetailPrimitives.swift:142–192；AutosavingMarkdownEditor.swift:5–18、30、163–195、221–409；NoonmarkStore+TaskMutations.swift:1832–1905；Sources/NoonmarkMacRuntime/IMETextDraftAutosaveGate.swift:99、112、167、190。

- TaskDetail 只在 trace pending 且 date>=today 时提供编辑器；只读标题用 MarkdownText，空标题 fallback 为“无标题”。默认中文；不自行接英文切换。
- 编辑器 style .title、showsSurface false、ownerID task:<chainID>:title；没有另加 Save/Cancel 按钮。
- commitsOnReturn 默认 false；该详情没有覆盖它。不能设计成按 Enter 才保存的单行表单，标题内部换行是有效内容。
- 普通非合成输入 700ms debounce；显式 flush 0ms；IME composing 期间不得写领域。失败由 gate 安排指数退避，最大 10000ms。
- onEndEditing / onCommit 请求 flush；onDisappear 走 lifecycle flush。退出有写入在途就等待，无超时，失败取消退出。不要以关闭详情就丢弃输入。
- 空白草稿在普通 debounce 时不调用 onPersist，保留本地草稿；完成编辑/flush 时恢复 lastAcknowledgedText 并 discardLocalChanges。
- 成功才确认 normalized 文本；新输入不能被较早完成的保存回调覆盖；不同 owner 的 session 必须隔离。
- store autosave 用 saveTaskTitleInput、background commit、publishesEngine:false，结束编辑等节点 publish deferred mutations；存在 automaticClassificationPolicy，当前 Go 空 catalog runtime 不可假装已复刻真实分类副作用。

## 必须进入自动化的测试矩阵

核心正例：
1. 无 traces 的池任务，两入口都原位更新，计数不变。
2. 今日/未来 pending：rename 分叉，saveInput 原位；所有 pending 重绑，date/order/notes/description/subtasks/status 不变。
3. 未完成历史+当前 continuation：首次 saveInput 分叉，历史标题不动；第二次 saveInput 复用新 ID，definition 数不再增。
4. 同一链跨多个 pending 日期：全部改绑；任一 pending 过去/锁日则整次拒绝。
5. returnedToPool 历史指向 current：核心 saveInput 分叉；runtime 暂仍拒绝载入该状态。
6. CRLF/CR、前后空白、内部换行、中文、Emoji、组合字符、Unicode whitespace；已有空标题有效，创建空标题拒绝。Foundation 与 Go TrimSpace 字符集差异必须真实 oracle 对账，不能假设完全一致。
7. normalized 相同：核心全图/时钟不变；但 abandoned/completed-only/过期 pending 仍先拒绝。
8. cancelledDraft 不计 formsDayHistory；current witness 分支单列。

核心负例：missing chain、abandoned、completed-only、unfinished-only、旧时钟/非法时间、重复/非法新 ID、sequence 极限；失败 before 深等于 after，传入 slices 不与内部 alias。

历史审计攻击：旧标题/description/plannedSubtasks/createdAt/sequence 偷改；仅推进旧 clock 无合法 successor；改已 superseded 版本的指针或时钟；跨链/现有 successor；错误 sequence；多造 definition；偷偷改历史 trace；新增已锁日 trace/subtask；伪造 prior；audit revision/events 修改；rich catalog 或 recurrence 混入。所有均拒绝，旧 strict CaptureEmptyHistory 行为保留。

service 持久化：正常保存重开、清空核心后重开、分叉后历史标题及 audit 原样重开；write/encode/audit 故障注入不发布；两 service 竞争只一个成功；旧详情 callback 与 AdvanceTo 竞争；重复相同 autosave不膨胀 definition（是否避免多写磁盘 revision另行明确定义，当前 transact 总会 commit，不等于核心 no-op有写入）。

UI 症状：输入非空 700ms 后 durable；中文 marked text 不提前持久化；清空并失焦恢复已确认值；有效多行标题；编辑中切换任务/关闭详情/退出不串线不丢失；失败显示/重试/退出取消；保存中继续输入最新值最终落盘；completed/past readonly；重开恢复。

## 证据等级与待补 oracle

本报告完成的是静态读与结构推理。官方测试明确覆盖可编辑范围、完成/未完成拒绝、历史 continuation 分叉后二次输入复用、已有空标题可存；它们在此没有运行。未验证真实 Swift 的完整 snapshot 差分、时间失败原子性、returnedToPool 编辑、Unicode normalize 字符集、restoration witness 后编辑、非当前 pending definition、IME/blur/Return/quit 的真实 App 事件路径以及 Go↔Swift v7 对账。应由允许的隔离 Apple audit/e2e 环境补充，不得探生产资料，也不能用自制 Go expected fixture冒充 Swift oracle。
