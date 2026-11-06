> 阶段归档：保留原设计或隔离副本验证边界，正式整合状态见 README。

# 待实施测试矩阵（未执行）

共同 oracle：成功必须 sealed result valid；whole-graph equals independent expected；classifications exact bytes/count不变；所有非豁免字段presence/order/identity冻结；changed取整snapshot差；错误返回zero proof且所有输入不变。测试通过不产生Service写权限。

| 编号 | 输入/分支 | 必须断言 |
|---|---|---|
| T01 | 池无day history，Rename/Input | 原ID和sequence保留，仅title/content与chain max变化；unused newID为空、bad、旧ID均可 |
| T02 | 只有当前/未来P，Rename与Input同标题变化 | Rename新版本且P转引用；Input复用，不重写P clock；非目标链冻结 |
| T03 | 当前定义被历史U/R/C/A引用且有可编辑P/R | 两action均版本化；历史trace完整不变；历史definition正文/description/children冻结，只有三元metadata变 |
| T04 | 首次Input已分离历史定义，再逐字符Input | 首次+1定义，后续reuse；不能每键+1 |
| T05 | 只有C/U/A等不可编辑事实、abandoned链、past P、locked P | 即使标题canonical equivalent仍分别拒绝transition/abandoned/immutable；trace owner历史亦拒绝 |
| T06 | route chain/trace，错chain、缺trace、past trace、非P、未来P | exact owner fence；未来P允许；chain池正常；rename+trace/未知route拒绝 |
| T07 | CRLF/CR、Unicode whitespace、空标题、NFC/NFD、长combining non-stream-safe | Go normalization与equivalence固定；canonical no-op保留stored bytes，无CGJ插入；非法UTF8拒绝 |
| T08 | no-op附unused Now = NaN/±Inf/更早/MaxFloat；newID无效/重复 | 不消费参数，仍full inspector/prefix；changed=false；metadata只代表request |
| T09 | 变化Now早于current；version早于pending；Now equal；chain有更晚note | 前两拒绝InvalidClock；equal可成功；chain不回退；reuse不增加并不存在的pending clock检查 |
| T10 | version invalid/duplicate newID、maxSequence溢出；restored current successor witness | 仅真实version消费ID/seq校验；恢复witness正确覆盖，新定义copy exact |
| M01 | Percent负数/0/50/100/>100、minInt/maxInt | clamp后结合floor；100仍P，不写chain；输入整数不溢出 |
| M02 | nil→0、已有0→0、floor=60请求20/60/100 | nil→0变化，已有0 no-op，低于floor仍60，zero capture |
| M03 | chain任一历史presentable child；仅cancelled child；别链child | 首项阻断，后两不误阻断；不得只看目标trace child |
| M04 | missing trace、wrong chain、past/future/locked、C/U/A/R/D/X | core错误次序；即使next相等也不能逃guard |
| M05 | earlier date/same-date lower continuationSeq floor；same group peer | 独立raw手算floor，不引入同组peer错误；回池再排期与延期/延续floor不丢 |
| M06 | manual 100→complete→undo；manual→settle U→合法续期P | 保留历史manual与floor；新helper只审本次manual，其他edge用其实际adapter或标清core-only |
| M07 | no-op未消费NaN/Inf/早时钟/MaxFloat；变化equal/早/nonfinite | no-op成功、equal变化允许、早/nonfinite变化拒绝；不声明Service同样接收这些wall clocks |
| H01 | 无事件/[A]P/[A,A]P；另一链[A,U]/[U,A,U] | title/manual合法目标均继续可用，整个旧流保持原byte/order/key大小写/instant bits |
| H02 | 新历史source为U而无P/R；current合法P在另一链 | title原guard拒绝该source；manual仅P目标可用，不编造可达状态 |
| B01 | seal零/错profile + huge graphs | seal先失败，所有后续gate不执行 |
| B02 | before huge+after malformed；before malformed+after huge；两图合法input huge | 分别Before/After/InputFields typed budget，早于bridge/validate/string scan |
| B03 | Title、ChainID、TraceID、NewDefinitionID各huge、共享string累计超限 | 每logical use计费；unused newID也有资源上限，但无UUID36要求 |
| B04 | bytes=N/N+1；shape=N/N+1；overflow arithmetic；近envelope Unicode | exact边界，不用6x估算误拒；canonical真正输出超限仍失败 |
| B05 | 未使用newID为短/非UUID/旧ID及UTF8任意字节 | 资源范围内 no-op/reuse可过；version由core拒绝；用真实当前head验证 |
| F01 | 改非目标chain/day/child/old definition、trace status/note/description/priority/manual | after replay拒绝；独立fence helper单测即使模拟replay缺陷仍拒绝 |
| F02 | 新定义不copy description/planned presence、旧reference改动、重排/额外新增 | 拒绝；nil与空slice按实际canonical可表达语义检查，不虚构wire区别 |
| F03 | prefix两端各合法但旧event改写/删除 | 单独prefix测试拒绝；不冒称audit能直接传分类candidate |
| A01 | 成功后修改caller shared pointers/slices/returned AST | owned endpoints不变；第二份snapshotCopy与第一份互不污染 |
| A02 | 各失败阶段、zero/unknown-kind result | 无有效proof，inputs exact unchanged；不提供generic constructor |
| G01 | AST四gate→bind、metadata helper白名单、finish(nil) | 新入口和旧入口全覆盖；旧断言不删；新budget无扫描/clone/validate |
| G02 | SHA+逐函数body冻结 | 只有enum/proof/注释/valid三case为例外；旧audit/core/budget/store/r1/v2无漂移 |

## Fixture谱系与测试隔离

H01用已有新纯生命周期adapters真正组成，不手补event以冒充命令生成。[A]P: seed→Abandon→Reactivate；[A,A]P: 再Abandon→Reactivate；[A,U]: P带A→跨日settlement；[U,A,U]: settle U→Abandon U→后日Reactivate U，并由相应adapter审计。每步要求完整inspect与prefix通过，且now选择有限严格递增避免把equal边界混入可达谱系。要测试manual目标时在同一完整graph预置另一条当天P，或以后已审计continuation生成P；不直接把U改成P。

所有pure参数fixture都单独标明非Service可达证据。为equal/MaxFloat/negative/±0用合法prior，仅改变本次参数，no-op允许nonfinite参数不代表snapshot允许nonfinite stamp。旧Service耗尽frontier no-op真实重开测试应继续跑旧套件；此批不改Service也不声称新writer重开成功。真实UI标题IME/owner切换/保存退出/失败草稿保留为未来接线验收，manual当前只读UI，不新增slider。

Swift oracle 后续只合成内存engine，不读生产；比较title等价no-op原bytes、两个mode版本数、R/P与历史引用、手动floor/child阻断。Apple Foundation空白集合差异属于待测边界，不能用Go单测假装oracle。各OS/arch构建和视觉验收另批。
