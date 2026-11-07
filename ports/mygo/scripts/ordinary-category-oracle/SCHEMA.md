# 普通单分组参考器输入／输出契约 v1

此文件描述待审查代码，**不是已执行的 Swift 对账结果**。

## 身份与范围

- suite：ordinary-category-only-v1；上游43e6eefaae3a04be89cab3e234d470111afd80ef。
- INPUT.json 原始 UTF-8 共6181 bytes，SHA256 2a3f43ffb5a78165ce9d32958089e0e3973280829fa5f3ab33f69fa44ea70aae。只接受这份 exact bytes，含128个固定 interaction UUID、10名称、14个 clock值、两条固定合成标题。UUID 是输入授权的确定性交互身份；引擎自己生成的 plan/category/nameVersion/record/removal/chain/definition UUID 完全不替换。
- clocks 是 reference-date Double 的16位十六进制 UInt64 bits，链创建时间也显式 supplied。Date 初始化、record payload 及回读都保留/对照实际 bits。额外两个无规范化来源差异：UI-reference 只把 D(raw) 作为 core 输入；prepared-create-reference 是独立研发用 standalone create，均不计为 ordinary 产品能力或UI实测。
- Foundation Codable 仅作用于 classification section、category、record、receipt 和本地 graph DTO（chains/definitions）。不调用 NoonmarkSnapshot Codable/validateIntegrity、不调用 Storage/DataPackage，不调用旧 suite runner、门禁或比较器。

## observations

完整采集为65观察：22个普通序列／负例、10名称×4参考入口、3日期位模式。这个数是本独立 suite 的采集数量，**不是业务通过数**，不加入既有1807名称或32快照分母。

主序列是 new 工程 → 新 interaction 再传相同 .new 工程 → 保留首个 plan 的原时钟 retry → 不同 now retry → none → none；第二 chain 为 none → existing 工程 → new 工程复用。主第二步重传原 .new choice，参考 prepare 应当返回 existing 的实际观察；不预制结果。其他链较早提交时间是在同一序列真实提交，原记录数组实际 canonical order 原样保存。

每观察文件 `<caseID>.json`：
- schema = ordinary-category-observation-v1，caseID 必须在固定清单且顺序一致。
- kind = command / single-field-state-mutation / normalization-reference。
- command 保存 request（原输入及UTF-8 base64）、prepare status、实际 plan 信息、bound-user confirmation的planID/interactionID/digest/decisionID、nowBitsHex、before、commit status、receipt原件引用、实际record及afterStages。
- prepare/commit/integrity/encode/decode 每阶段 status = ok/error/not-run。error带原类型和最多2048字符的诊断；not-run带原因。领域拒绝只是观察，不自动通过或失败整个 suite。harness/来源/摘要/预算错误独立终止整个采集。
- stateMutation 从 `main-none` 的真实有removal正控制单点突变；before由正控制提供，afterStages单独观察。**missing-removal 可能暴露适用 integrity 与 replay 因果证明的差别，不强迫 Swift 拒绝。**
- normalization direct保存 raw、D0…D5 与 K(Di)、原category/nameVersion IDs、实际实体name/key、category encode/decode。prepared/core/UI的实际request、preparedIntent、preview来源（plan payload）、state等在各自观察。无任何UI验收结论。
- stateView是辅助直接字段投影，包含revision十进制字符串、category原name/key文本+UTF-8 base64、原NameVersion IDs、空current键、原始record/removal数组ID顺序、receipt interaction ID集合及以原ID为路径的所有日期bits。排序仅用于无序集合的辅助投影，不重排原始 Foundation bytes 或审计数组。

## 原件与摘要

每原件reference：path（平面 ASCII 文件名）、bytes、sha256、role。

`reconstructedPayloadBytes`：分别按上游TC私有payload结构重建的Encodable material，字段类型来自实际返回plan/record。plan、record引用分别带实际digest；记录SHA256复算匹配，并调用原hasValidDigest/hasValidIntegrityDigest。它们**不是截获的私有encoder原件**，必须保持此标签。

plan material精确字段：formatVersion:Int=1, planID:UUID, baseRevision:UInt64, interactionID:UUID, source:ClassificationSource, changes:[ClassificationPlanChange], createdItemID:UUID?, plannedCategoryID:TaskCategoryID?, plannedLabelIDs:[], notices:[ClassificationNotice], blockers:[ClassificationBlocker]。optional nil由Swift合成Encodable省略。

record material精确字段：formatVersion:Int=1, id/planID/interactionID:UUID, source, decisionID:UUID?, changes, notices, committedAtBitPattern:UInt64, revision:UInt64, planDigest:String。UInt64为实际Date reference interval bitPattern，非Date、毫秒或浮点JSON。

所有 Foundation 编码使用 JSONEncoder sortedKeys、默认deferredToDate；原state/record/receipt/category bytes在decode或任何辅助投影的跨引擎映射之前写入。state/category raw输出与重建payload分开。负零或其他 Date 默认编码回读改变位时，decode status、stateEqual与allDateBitsEqual分开，不丢弃原bits或修改值修复。

post-prepare故障注入的preview/digest修改均明确标记，只有 `preview-tamper-old-digest` 和 `digest-tamper` 可有 digest不匹配；它们不是原prepare黄金结果。rehashed-preview使用实际ClassificationPlan initializer计算新digest，再由真实commit检查intent一致性。

## 完整性清单

manifest.json：schema ordinary-category-artifacts-v1、suite、upstreamCommit、inputSHA256、captureComplete、semanticParityVerified=false、runtimeAdmission=false、fullSnapshotExecuted=false、causalIDNormalizationApplied=false、algorithmVersion、observationIDs、artifactBytes、files。

captureComplete只表示本次真实进程成功走完固定采集且原件完整。它不表示所有领域输入被接受、所有integrity通过或Go语义匹配。Python runner复核全部原件SHA256、精确观察ID清单、两个重建payload的hash与实际digest；不能依赖观察器自报matchesActualDigest。

runner receipt也保持semanticParityVerified/runtimeAdmission=false。没有Swift实际执行时，不会产生manifest/capture-receipt。static verify报告只含staticSourcesAndToolsVerified、swiftExecuted=false、semanticParityVerified=false。

## 限额与失败

输入<=16KiB且exact hash；单原件<=256KiB，观察JSON<=64KiB，至多512原件，总16MiB，清单<=128KiB。每个文件独占创建，输出目录初始必须为空。Swift命令只有两个路径参数，输入不能指定动作或额外文件。runner只编译固定清单，不下载资料。

运行stdout/stderr各64KiB且逐块计数；编译共享900秒，执行60秒。超限/超时杀进程组，不重试。完整pipe保存*.raw；异常路径的prefix只标*.diagnostic-prefix，不冒充完整原件。失败保存有界failure.json/runner-failure.json和已完整写下的原件，不生成成功receipt。普通领域decode失败仍保留已成功encode的完整原bytes。

`upstream/`内所有15文件SHA256/Git blob/长度与SOURCE-CLOSURE.json核对且禁止额外文件；runner锁定闭包SHA。TOOL-MANIFEST.json列出本地工具/文档，capture需要独审给出的manifest SHA，避免任意未审工具生成被当作已审输出。未来macOS官方镜像/Xcode可用性还需外部核对；代码只检测实际Xcode26.2/Swift6且记录平台，绝不自报官方hosted镜像已核实。

## 完整性与执行身份约束（优先于前述简要说明）

- verify_capture先核受信INPUT原bytes和输出input-original的同一SHA，固定65观察不从未核输出推导。每个variant、阶段status/错误详情/not-run原因、成功阶段的必需raw reference及其path/role/bytes/hash逐一验证，observation真实上限64KiB；名称D/K及prepared intent子结构不可缺失。plan intentional tamper只允许固定两例，实际reconstructed hash必须与标记一致。非空algorithmVersion必需。终态复核所有输出文件身份。
- logicalAndDateBitsUnchanged替代原stateUnchanged，只表示领域Equatable加日期bits相同，不声称UTF-8或JSON原bytes全同。原件继续保留。
- 十一个普通positive controls必须实际commit、integrity、encode、decode全部成功后，才进入passedControls。每个dependent probe有固定controlRefs，先检查前置成功；失败观察先完整写出，再终止capture，不生成成功manifest。名称参考/negative probe不被强迫通过或拒绝。
- Python verify_capture单独只能确认protocol原件完整，永远sourceBoundExecution=false。只有capture实际运行成功、所有process group清理确认、已审stage身份终态不变，runner才给sourceBoundExecution=true。最终receipt绑定outputManifestSHA256、reviewedToolManifestSHA256、sourceClosureSHA256、inputSHA256、stagedIdentitySHA256及八个实际command receipt哈希。
- 在任何子进程前，精确已审15-source+工具/input bytes复制到全新private work/input，文件444/目录555，再核manifest并记录dev/inode/mode/nlink/uid/size/mtime/ctime身份。所有compile、runtime只读此stage，前后重复核验；live候选变化不会进入执行。根/父目录/leaf通过O_NOFOLLOW openat式descriptor遍历，禁止symlink及多hardlink，读取期间也核文件身份。允许系统/Xcode和private work，不再授权读HERE。
- 所有preflight（xcodebuild/xcrun/swiftc --version/sw_vers/SDK）同样受deny-default、deny-network与private HOME/TMP sandbox约束，无未说明的预检例外。
- Popen之后selector创建、注册、fd设置、轮询、退出、log写入之前均受lifetime guard。成功、nonzero、超时、超预算、setup异常路径全部kill process group并有界wait/确认group消失；cleanup uncertainty不能出成功receipt。完整pipe仍标raw，未完整pipe只能标diagnostic-prefix。
- 失败不自动恢复或重试。test_contracts.py 只做 Python 协议／文件身份检查与 mock 进程测试，不是 Swift、macOS sandbox、Go 或 UI 实证。CI 只上传明确允许的有界证据，具体规则见相邻 ../ordinary-category-oracle-ci/README.md。
