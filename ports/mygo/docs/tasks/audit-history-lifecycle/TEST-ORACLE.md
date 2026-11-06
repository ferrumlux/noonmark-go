归档说明：以下为冻结设计或独立阶段记录。状态和路径指当时阶段；原文 SHA-256 为 `16bec4700f9685b7be9a1ff6e09218177596044445237e5925e1949207eeff77`。整合进度以本目录 README 为准。

# 拟实施验证与独立复核清单

本文件仅设计测试，未建立/运行Go或Swift测试。正式基线 `3ea3efea`。必须先复核DESIGN，再隔离实现；不把下列预期当成通过证据。

## 1. Oracle构造

优先从core `CreatePoolTask`、`ScheduleFromPool`、`SettleTaskDays`、`ContinueUnfinishedTrace`、Abandon／Reactivate等真实API合成task事实。每步检查core error；完整aggregate prior必须实际通过fixed inspector。建前态时显式按来源capture合同构造分类期望，不调用待测新helper生成自己的oracle。可复用已有builder创建fixture，但独立断言完整expected event字段、old prefix、count、latest与原supplement。

ID使用固定规范UUID且全局distinct；Instant用 `InstantFromSeconds`检查error，不能整数强转bits。日期可用2026-11-05、06、07，Now用100/110/120/130；有来源例外的fixture显式注明结构合成，不能冒充Service可达历史。

每个正例至少比较：完整预期TaskGraph（测试独立手写许可delta）、完整canonical supplement、每旧event字节/顺序、每新event status/traceID/id/capturedAt exact bits/revision、changed=true、proof scalar identity、每次返回copy独立。不能只比较目标status、revision或event数量。

## 2. 来源正例矩阵

| 路径 | 独立预期 |
|---|---|
| 今日P→A | 唯一A@Now，chain abandoned；days/children/defs不动 |
| 未来P→A | 同上，无Today限制；future日不新增lock |
| locked U→A | 旧U保留，追加A，SettledAt替换为新A Now；子任务原样 |
| open U→A | 先以来源A→past U构造合法前态，再追加A，不能伪造无event U |
| historical请求，另有同链future pending | 改pending实际target，capture到实际target；请求历史trace完整不动；proof保存两种身份 |
| historical请求且无pending | 仅请求U可废弃；合法多unfinished形态下仍不选别的U |
| A→P current/open | 零EventIDs；同一TraceID；清SettledAt，旧A/latest/revision全部不动 |
| A→P future/open | 同上；不生成新day/trace/subtask，不消费allocation map |
| A→U past/open | 恰一U@Now，旧A SettledAt保留；open day仍open，子任务不结算 |
| A→U locked | 恰一U@Now；lock优先于Today比较，旧lock bits原样 |
| 多轮A→P→A→P | 每Abandon追加A；每P Reactivate零capture，完整旧prefix保持 |
| `[U,A]→[U,A,U]` | 新U的At为reactivation Now，settledAt为紧前A的At |
| A→past U→首次settlement | 用accepted settlement helper；新增day lock但不重复U，原stream/latest不变 |
| A→P→complete→undo | 已accepted helper继续可用，零capture保留A；之后complete→lock只加C |
| 无关链已有A/多事件 | 目标合法操作不受旧v2 gate干扰；完整graph仍精确不动 |
| 内容丰富 | children含pending/completed/cancelled、notes tombstone、planned children、manual progress/description/priority/lineage，全部原样保留 |

locked当前/future相对于调用者Today的纯参数组合，不是当前Service时序声明；若fixture无法由合法来源／完整profile构成，记录实际先失败点，不改validator造正例。

## 3. 负例矩阵与准确拒绝层

- prior零值/错seal：proof失败；两合法端点但before不匹配（无关chain/child/title/note/array顺序）：binding失败；prior任务rawValue归一化导致round-trip变化：binding失败。
- malformed/35/37/巨长IDs：metadata阶段；36字节非UUID：内容阶段；大小写重复已有event ID：builder拒绝；新增ID冲突不能改旧event来解决。
- Reactivate EventIDs长度>1：IDCount且不访问元素；P给一条、U给零条：Count；U恰一但invalid/重复：对应后续门禁。Abandon不接收可变capture列表。
- wrong owner在Abandon实际pending路由前拒绝；missing request、missing chain、state inactive、无pending的C/R/D/X请求失败。不能把wrong owner改为actual target owner。
- Reactivate仅请求A且chain abandoned；请求旧U、另一个A、active chain/已有pending、重复reactivation均按真实core/graph验证失败，不能重定向到其他trace。
- NaN/±Inf、局部trace或chain回拨拒绝；其他chain普通较新clock不能被误当global frontier；新增A/U早于旧global event则完整profile拒绝。
- U after保持旧A却不追加U：显式构造完整端点，inspector拒绝tail A；helper自身U缺ID由Count拒绝。P附加U不合法；不可为注入负例开放capture callback。
- Abandon错capture trace/status/clock，Reactivate U把SettledAt更新为Now而非保留A：分别以独立完整候选/typed after篡改证明拒绝；helper不接收的分类字段不能声称是直接入参攻击。
- 可通过core Validate的恶意after：改无关chain/trace clocks、定义、planned children、actual child、notes/tombstone、progress、priority；增删对象、改DayID/TraceID、复制子任务、重排array、改lock。全部exact replay或完整inspector拒绝。
- 两端各自inspector合法但旧event ID/revision/At改写、删A、stream重排／重编号：单独prefix拒绝。helper必须始终执行finish及prefix，不能拿after-only inspector当命令证明。
- 非empty catalog、非neutral prefs、ideas/series/recurring/changed：原scope拒绝。不得用fresh supplement替换旧段。
- future P→A→P→Return X：保留A，完整profile拒绝X+A。Swift完整snapshot源码同样拒绝；不是要求放行、已复现缺陷或当前v2 Service链。

## 4. 日期、时钟与容量

- Today min/max合法、零日期/非法闰日拒绝；date=9999-12-31 open且Today=max可A→P，不能虚构后一天/lock作为U来源。普通past/open与locked分支分别验证。
- negative finite、小数、subnormal、相等Now、signed zero、MaxFloat：比较数值合法性与赋值bits分开；成功不代表Service frontier可推进；目标bits翻转即使数值相同也需精确replay拒绝。
- before与after分别运行每个top-level/nested结构超限、raw bytes超限、多个小owner累计超限、共享slice/string/optional指针逻辑重复计费。预算失败与domain-invalid同时存在时必须先预算error，零结果且原输入不变。
- 复用并扩充已有schema manifest和各optional节点成本测试；新增入口AST顺序清单必须包括prior.valid、before budget、after budget、ID gate，之后才允许bind/FromDomain/Validate/clone/index/content。
- 极值checked arithmetic无需大分配；N/B下界临界通过但完整envelope超限仍拒绝；builder event增量、stream新增、revision溢出及final深度/bytes全保留。
- 新输入ID gate必须count先于元素访问；长度gate先于UUID扫描。模型gate不按字符串内容扫描／转义，不引入reflection到生产路径。

## 5. 不可变性及闭合身份

所有失败阶段（input、binding、core replay、after mismatch、append、full inspector、prefix）比较prior/before/after/EventIDs原样。成功后修改caller before/after的全部shared leaves、替换EventIDs[0]、修改snapshotCopy的嵌套值，proof及另一份copy均不得变化。只承诺调用期间输入只读；不以race测试暗示caller并发写安全。

共享immutable prior并发审核不同命令和获取copies，后续race门禁应通过。零result不能取snapshot/changed；unknown kind无效。检查proof的kind/Now/Today/ChainID/requestTrace/effectiveTrace/EventID与此调用精确一致，禁止返回上一命令结果。Abandon请求不同历史trace但同actual target仍必须保存不同请求身份；不能仅以after等价认定同请求。

## 6. 最小实施与验收计划

1. 独立设计复核：重点是0/1来源合同、Abandon实际target、metadata前置、closed scalar identity与全图freeze，不复制本设计的结论当来源。
2. 隔离新增 `history_lifecycle_command_vnext.go` 和对应来源／adversarial／budget tests；在现有result仅加两个closed kinds及scalar字段，复用现有bind/match/finish/预算。不得修改NFA/profile/core/Service/store/UI。必要的共享result和AST test扩充是明确最小影响面。
3. 实施前固定exact base/hash；先建立正反例，再实现。外置BUILD、离线依赖、低并行；本设计不新建源码副本/cache，实施资源方案另由整合任务确认。
4. 作者后续运行focused lifecycle、既有history命令、taskapp、race/vet；独立审查另写来源正例及恶意whole-graph delta，不借用作者oracle。任何失败回到设计，不静默缩域或放宽profile。
5. 独立审查通过才整合；精确整合树再跑原scripts/check、既定六目标cross-build/build/CLI与静态无接线diff。这些均为未来要求，本轮未运行。无Swift/非Linux实机证据即明确未验证。

本批完成用语限定为“未接线typed lifecycle审计在固定profile下通过隔离验证”。不能称v3 writer、持久事务、全消费者迁移、全图历史可达、全局时钟或Swift parity完成。
