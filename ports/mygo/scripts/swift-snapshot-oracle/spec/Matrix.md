# R3冻结矩阵及最低有效集合

状态全部not-run，本次没有parser、harness或runtime验收。R1矩阵原件保留于R1/Matrix.md，不能声称该全范围已完成。

## M1必须跑：18 case /32 point

SeedRegistry-R2.json是唯一case动作清单，每个case还必须有seed观察点。所有seed目前pending-validation；两端真验证前不标valid。每个观察点的检查固定七层（完整entity、array-sequence、raw package、raw export、双端self-roundtrip、双向cross-decode）。任何一格无效或unsupported都使M1 observation未验收，而不是调整分母。

- observe-empty：empty，无动作。
- observe-planned-12：planned-12，无动作。
- observe-planned-21：planned-21，无动作。
- observe-multi-interleaved：multi-interleaved，无动作。
- schedule-planned-12：planned-12→schedule-one（新day，2children）。
- schedule-planned-21：planned-21→schedule-one（合法[2,1]，lineage-position与上一格对偶，不重排after）。
- schedule-multi-interleaved：multi-interleaved→schedule-one（既有day，旧2trace各2children交错、新trace共存，priority跨chain）。
- create-nil/create-empty/create-ascii/create-nbsp/create-figure/create-nnbsp/create-ideographic/create-nel/create-zwsp/create-bom/create-valid：empty→create-one，精确输入和独立Go适配见NoteVectors-R2.json。

四seed所有UUID和数组序都在JSON中明确。planned-12与planned-21只排列原序不改字段。multi seed原trace列表与priority次序相反、旧children跨owner交错；因此不能只对新tail排序。seed代表boundary构造，不代表已有产品命令轨。空classification所有数组也有完整投影/编码检查；不能因空而省略字段。preferences.sync enabled=true，export投影=false，能看出少做投影和误改原snapshot。

## R1覆盖去向（保留全部缺口）

- A01：M1只取create/schedule部分；addPlanned命令可达性延期M2，不能由seed替代证明。
- A02/A03/A04：M1对应planned对偶和multi-interleaved。
- A05：M1 multi链基础已观察，但专门ID与creation逆序对偶延期M2。
- A06/A07：旧definition、新版本、活跃多note/deleted tombstone均M2/not-run。M1只单活跃create note。note排列差异门禁亦明确M2/not-run，不列入M1 V03必跑子项。
- A08：M1只空classification集合形状；非空dictionary pair order是M2/not-run。
- B01/B02：非单调Now/Date专门对偶M2/not-run。
- B03：M1只跨chain正priority，负priority、X排除、其他date候选M2/not-run。
- B04/B05：受控同ID tie和随机tie专门格M2/not-run；M1仍保留随机IDs导致原bytes不同，但不宣称完成tie验证。
- C01：M1新day/既有day；review/locked等非空optional M2。
- C02/C03：±0/相邻值/负值/MaxFinite/非finite/精确Date边界M2/not-run；M1只固定有限100/101 reference秒，并保留所有输出bits。
- N01：M111个精确向量；不代表完整Foundation/Go Unicode集合等价。
- N02：M1 valid正文覆盖quote/slash/emoji/组合字符；description nil/empty/value完整对偶M2。
- N03：explicit blank note的domain原错误测试M2；V负例不把它算Swift互通。
- H01/H02/H03/H04：全部M2/not-run，零history M1没有return动作或taskapp adapter；多R/X事件stream、sealed prior/audit真实入口必须另设计冻结后跑。

## V资源及比较器门禁（所有必须pass才M1观察验收）

每项是在后续工具实现中执行的负例规格，现在未运行。比较器负例使用成功合成capture的单点突变，失败样本不计有效业务point。

- V01：每compiled/evidence/tool文件SHA改一位、missing/extra/duplicate manifest path、错误commit/root/Core/Storage/App tree→compile前拒绝，compile call=0。
- V02：重复binding ID/key、缺trace lineage、owner改错、额外实体、同一命令有两个definition候选→mapping-ambiguous或schema-invalid；不按数组index补对齐。
- V03：分别只置换顶层trace、旧children、planned原序；entity关系可equal而array必different。仅改Position/owner/lineage/clock/optional/UTF8任一值，entity必different；不能用一层绿覆盖另一层红。
- V04：package原bytes插空白、改escape、key顺序、array顺序；raw必须different，双向真实decoder实际返回独立记录。不在cross前重新canonicalize。
- V05：遗漏case/point/Stage/Binding/Observation/Comparison字段，重复comparison layer，缺artifact/hash错，negative result挪入effective，unsupported算通过→controller拒绝report统计。
- V06入口覆盖：stdin、registry、vectors、每个seed、每个direct projection、每个实际package/export/self-reencoded、双向cross输入、每个capture、source manifest、guard evidence、stdout/stderr、artifact总数/每case总bytes/全run总bytes，逐个测limit+1。每类同时测byte/depth/node/string累计/单string/array/object-member上限，适用hex测奇数、invalid UTF8、decoded+1、2×扩张，Base64字段无条件未知拒绝；pairs测奇数/重复键/非空profile。参数枚举以FixtureSchema每行所有限值笛卡尔单独覆盖，零上限array用1项。每次有decoder/clone/bridge/command/serializer调用计数见证：对应前置失败后下游计数=0。不能用总stdout限额替代逐字段测试。
- V07：create/schedule预计增长超过实体/node/text/output保留额度，调用真实command前拒绝；上下游未进入见证。shader/模块/Swift工具阶段限时，stderr持续输出总额超过1MiB中断并最多64KiB尾段；不能缓存全量再测。
- V08：至少3次同seed运行，实际schema/映射/bytes全部记录，不把随机IDs/dictionary变化抹平。故意把不同raw hash改成equal，comparator必须发现。只有首轮计固定32 point，重复轮不增加分母。
- V09隔离：guardEvidence的check白名单恰 source-root-new、source-no-symlink、paths-no-overlap、compile-network-denied、runtime-network-denied、runtime-production-denied、runtime-icloud-denied、runtime-keychain-denied、sdk-read-allowed、artifact-allowlist、runtime-home-isolated、runtime-temp-isolated；全部pass才允许scope=false。使用虚拟哨兵/策略检查，不去探测production或真实iCloud/Keychain目录。source fetch受控唯一联网，编译/运行无真实AI。任一证据缺失或写出allowlist外→harness-error。

## 状态统计必须可复算

所有18case给最终status且M2格可见，仅reportComplete。M1 observationAccepted最低32/32 effective point+V01–V09全部实际pass，无invalid-fixture/unsupported/ambiguous/not-run/harness-error。fixture拒绝不能算该格有效覆盖；如果seed实际不合法，修订fixture和hash并独审，不改分母或以负例成功顶替。interopAccepted还要求32 point七层全部equal；raw随机身份不同会阻止该结论，但不阻止诚实difference观察。仅比较器测试通过与业务互通是两类计数。

R3的V03必跑排列子项仅顶层trace、旧children、planned。note长度0/1无法产生排列差异，禁止新增多note fixture扩大M1或以profile拒绝算array差异通过。V06/V07还须证明限内malformed真实raw先安全存档、hash正确且状态malformed-raw；schema失败不调用下游decoder，不删除既有raw差异；超限仅有界诊断、无截断原件。18case/32point不变。
