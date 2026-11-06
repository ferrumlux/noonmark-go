# R3有限schema、预算与阶段状态

本版在R2封闭规格上作窄范围修订；协议schemaVersion=2和R2命名的固定输入JSON不变。FieldSchema-R2.json、SeedRegistry-R2.json、NoteVectors-R2.json及r2-seeds/*.json都是规格附件，不是运行capture。JSON未知key/重复key/错类型一律拒绝，输入不可携带代码、路径、URL、环境变量或自由checks。

## 1. 输入精确结构

运行stdin只有 {schemaVersion:2,registrySHA256:SHA,caseID:Key}，caseID必须匹配18个registry case之一；不接受调用方覆写seed/command/IDs。负例测试用独立固定mutationID（V矩阵定义），不能增加运行协议自由payload。registry根固定{schemaVersion:2,seeds:[Seed],cases:[Case],checks:String,snapshotCountPerCase:String,allSeedIDsFixedAcrossSides:true}，其中三说明字段必须等于附件原文。Seed={id:Key,file:RegistryFile,sha256:SHA,validity:"pending-validation",origin:固定说明}；file仅4个硬编码相对文件名，禁止路径穿越。Case={id:Key,seed:SeedID,steps:[Command],required:true}。注册顺序只是呈现，不用于身份对齐。seed描述为完整Envelope，字段类型及允许值见FieldSchema-R2.json；不是从capture反生成。

Command严格二选一：
- create={op:"create",commandKey:Key,chainGoID:UUID,definitionGoID:UUID,noteGoID:UUID,titleUTF8Hex:Hex,description:{present:Bool,utf8Hex:Hex},noteVectorID:NoteID,nowBits:Bits16}。
- schedule={op:"schedule",commandKey:Key,chainRawID:UUID,traceGoID:UUID,dayGoID:UUID,date:LocalDate,today:LocalDate,nowBits:Bits16,childrenGoIDs:[{lineageRawID:UUID,id:UUID}]}。
实际值必须逐字符合registry，不仅通过类型。没有return动作、输入identities、checks或额外命令。

NoteVectors条目={id:NoteID,input:{present:Bool,utf8Hex:Hex},goInitialNote:{present:Bool,bodyUTF8Hex:Hex},swiftExpected:固定说明}。present=false时hex必须空；true允许空。Hex小写偶数，先检查hex字节预算，再解码、验证UTF8；不规范化Unicode。具体NUL/空白与正文来自附件的精确hex。nil/empty/ASCII/NBSP/U+2007/U+202F/U+3000/U+0085预期Go nil；U+200B/U+FEFF是非空正文；valid使用附件中的中文、quote、slash、emoji、组合字符。Swift实际不同则保留difference，禁止更新预期让它相等。

## 2. M1值类型和字段全覆盖

FieldSchema-R2.json objects每条{wire,type,optional,swift,go,m1}为封闭字段映射。Swift字段同wire名；Go core首字母大写/ID保留（例definitionID→DefinitionID），相关实体类型在文件明确；非core section通过Supplement canonical.Value保持原结构，不伪称有独立Go实体。Envelope为instrumentation包装，Snapshot为Swift NoonmarkSnapshot/Go recurrence.State+Supplement；ID.rawValue为Swift UUID/Go typed UUID string，不创建Go.ID struct。LocalDate年/月/日各32位整数；uuid是36字节大写规范格式；stamp是无前导零UInt64整数词法，finite，比较bit而非decimal秒。

可选属性只允许缺省或类型值，不允许null；M1除descriptionText以外都必须缺省。负例输出遇null记录为profile违规，不能删掉它重新比较。description absent或string，包括empty，均保留。整数普通值限signed64，difficulty∈1,2,3；sequence=1、continuationSeq=0；priority普通signed64（但实际registry固定，非registry负priority属M2）。Chain.State=active，Trace/Subtask.Status=pending。Note.deletedAt absent，note body为非空规范正文假设，实际两端验证为准。

所有schema标empty-only的array必须存在且长度0，包括taskCycleSeries/ideas/syncEndpointOptions及全部classification集合；元素类型不在M1实例化，非空立即unsupported-profile，不能先解码未知结构。classifications formatVersion=1、revision=0。preferences及其nested policy固定与每seed一致，只有export projection的enabled由true变false。Task arrays允许由create/schedule增长，其他字段不自由输入。typed projection也服从同一字段schema，不另有可忽略fields列表。

## 3. 输出精确结构

每case独立capture文件，根={schemaVersion:2,caseID:Key,registrySHA256:SHA,sourceManifestSHA256:SHA,toolchain:Toolchain,scope:Scope,fixtureValidity:Validity,points:[Point],summary:Summary}。任何缺key拒绝。

Toolchain={swiftVersion:String,goVersion:String,xcodeVersion:String,sdkVersion:String,osVersion:String,architecture:"arm64"|"x86_64",target:String,locale:"en_US.UTF-8",timezone:"UTC",runnerCommit:SHA40,runnerSHA256:SHA,harnessSwiftSHA256:SHA,harnessGoSHA256:SHA,comparatorSHA256:SHA,compiledSourceManifestSHA256:SHA}。版本字符串各≤256B，target≤80B；未知runtime不填空值，stage harness-error。source manifest为另件有界固定结构，见§5。

Scope={productionDataAccess:false,iCloudAccess:false,keychainAccess:false,aiNetworkAccess:false,actualAppExecuted:false,selectedStorageSources:true,guardEvidenceSHA256:SHA}。false必须有guard evidence，缺失则harness-error。

Point={key:"seed"|"create-one"|"schedule-one",stages:[Stage],bindings:[Binding],observations:[Observation],comparisons:[Comparison]}。固定seed点，加至多一个命令点；失败后point只记录真实已发生stage和其余not-run，禁止假after。

Stage={side:"swift"|"go"|"controller",name:StageName,outcome:Outcome,reason:Reason}。StageName封闭：preflight,seed-prepare,seed-decode,seed-core,command,snapshot,from-domain,encode-package,encode-export,self-decode,self-encode,cross-preflight,cross-inspect,cross-decode。不适用阶段outcome=not-applicable（如swift from-domain），reason=not-applicable，不能当成功增加coverage。Outcome=success|rejected|unsupported|not-run|harness-error|not-applicable。Reason=none|resource-limit|schema-invalid|registry-mismatch|invalid-fixture|profile-violation|domain-rejected|codec-rejected|mapping-ambiguous|dependency-failed|missing-evidence|timeout|source-mismatch|not-applicable。SDK错误原文不作稳定reason，不含自由用户内容。

Binding={kind:Kind,key:Key,swiftID:UUID,goID:UUID,origin:"seed"|"create-one"|"schedule-one",rule:Rule,ownerKey:KeyOrEmpty,lineageKey:KeyOrEmpty}。Kind=day|chain|definition|planned|lineage|trace|subtask|note。Rule=seed-id|returned-chain|new-owner-definition|new-owner-note|returned-trace|date-day|trace-lineage。seed key为kind+"-"+UUID.lowercase，故Key一般上限80（输入case/command≤64）；command key为kind+"-"+commandKey，children key为"subtask-"+lineage UUID.lowercase。lineageKey只给planned/actual，owner给definition/planned/trace/subtask/note，day/chain/lineage为空；未完成映射不得输出半条Binding，reason=mapping-ambiguous并阻止比较计数。Raw UUID不用于排序建立关系。

Observation={layer:ObservationLayer,artifact:Artifact,rawState:RawState,reason:Reason}。RawState=archived-unchecked|schema-valid|malformed-raw；存档后未检查为archived-unchecked，通过scanner/schema才为schema-valid，限内但scanner/schema拒绝为malformed-raw并保留完整原件。最终成功point不得仍有archived-unchecked；malformed-raw不能作为成功观察或effective point，原bytes已知差异仍可记录。ObservationLayer=swift-engine-projection|go-kernel-projection|go-from-domain-projection|swift-package|go-package|swift-export|go-export|swift-self-reencoded|go-self-reencoded。前三为Envelope字段投影JSON，后六为原始真实serializer bytes。Artifact={name:ArtifactName,bytes:UInt,sha256:SHA}，name由controller生成caseID/pointKey/layer.json固定白名单，每组合只一次，exclusive-create，无自由路径。投影完整内容放有界原件，schema如§2；不输出无界fields/arrays的另套重复表示。原始array顺序从直接字段投影保存，不由实际encoded bytes反推；比较器按完整schema遍历所有数组。不存在Base64字段，任何Base64编码原件/字符串wrap作为未知字段拒绝；不允许使用base64膨胀绕过raw预算。

Comparison={layer:ComparisonLayer,verdict:Verdict,reason:Reason,firstPath:String,firstOffset:Int,leftSHA256:SHAOrEmpty,rightSHA256:SHAOrEmpty}。ComparisonLayer七项见Design；Verdict=equal|different|rejected|not-comparable|unsupported|not-run|harness-error。firstPath为≤256B的schema路径（只含schema字段、逻辑key和首差index），无差异为空；firstOffset=-1表示不适用，否则原件内offset。digest缺原件为空，不能有虚构digest。数组比较可以用index报告首差，绝不能用index绑定实体。对每point恰7条comparisons，任何缺/重复layer为harness-error。cross-decode verdict equal表示双向均接收且解码后identity-mapped全字段/数组关系符合各自原包，而不是宣称raw bytes一致；任一拒绝verdict rejected。

Summary={fixtureValidity:Validity,mapping:"complete"|"ambiguous"|"not-run",effectivePointCount:UInt,interopPointCount:UInt,negativeGatePassCount:UInt,reportComplete:Bool,observationAccepted:Bool,interopAccepted:Bool}。不得相信子进程自报统计，controller重算。Validity=pending|valid|invalid-fixture|unsupported|harness-error；完整seed两端所有验证和true encode/inspect/decode完成才valid。拒绝种子即invalid-fixture，后续命令not-run。不同步输入拒绝仍是观察事实，但非有效业务比较。

## 4. 前置预算：所有入口同一规则

各上限均硬边界，不是申请内存的建议；checked加乘，不预按不可信长度分配。固定缓冲预检允许有界分配，不宣称零分配。JSON scanner流式统计depth/root=1、每value与每object key各一个node、UTF8有效性、重复key、integer词法、每字符串与每数组长度，拒浮点/非finite JSON数字、pair奇数及重复key身份。预算通过后才JSONDecoder/JSONSerialization/canonical.Parse、反射、构图、Clone/bridge/validate。source认证输入也先限额。

- stdin invocation≤1KiB、depth≤4、nodes≤16、每string≤80B，无array。
- registry≤64KiB、depth≤12、nodes≤4096、string≤256B（hex≤8192字符），全string累计≤32KiB；seeds恰4、cases恰18、steps≤1、childrenGoIDs≤2；没有identities/checks数组。vectors恰11，≤16KiB、depth≤6、nodes≤256、hex≤8192字符且decoded≤4096B，累积decoded≤16KiB。
- 任意seed、projection、package、export、self-reencoded、cross输入：单件≤256KiB、depth≤24、nodes≤8192、string decoded≤4096B、所有string累计≤64KiB；object members≤32；顶层days/chains/definitions/traces/subtasks分别≤16；每definition planned≤2；每owner note≤1；全部实体出现计数（含planned和note重复出现）≤96；所有空array强制0；其他未知array拒绝，故dictionary pairs在M1必须0（结构扫描仍先核偶数）。typed projection对optional/type同样检查，不能只看包总长。
- capture≤128KiB、depth≤16、nodes≤8192、一般metadata string≤256B、name≤160B、key≤80B、sha64/commit40/uuid36/hex16固定；累计string≤64KiB。points≤2，每point stages≤32、bindings≤64、observations≤9、comparisons恰7。所有array都有此封闭上限，不存在自由rawIdentitySequence/logicalSequence/fields数组。artifact count≤18/case，总raw bytes≤4MiB/case，18case总raw≤72MiB；捕获JSON总≤2304KiB。总证据（raw+capture+metadata+logs）≤80MiB，先预留预算再创建每件，失败不写半件成功manifest。
- source manifest≤128KiB、depth≤8、nodes≤4096、files≤64；每row固定{path:String≤160,role:"compiled"|"evidence"|"tool",gitBlob:SHA40OrEmpty,sha256:SHA,bytes:UInt≤4MiB}，重复path拒绝。编译源必须恰14Core+2Storage；metadata文件Package/LICENSE/App只能evidence。guardEvidence≤64KiB、最多64条固定{check:Key,outcome:"pass"|"fail"}；失败不得scope宣称隔离成立。
- stdout每进程≤128KiB（仅capture），stderr边读边保留最多64KiB尾段，总读超过1MiB即终止，不能先无界PIPE。每artifact限额在写入前分块计数，serializer Data在内存产生前先作state预测预算，产生后立刻检查限额再落盘。每case运行≤120秒、编译≤900秒，超时/超量杀进程记harness-error。

强制顺序：限长读取→token结构预算→精确schema/registry匹配→引用与预计增长预算→domain decode/construct/clone→实际command→snapshot投影state预算→bridge→serializer→单件byte/件数/累计byte安全存档预算→固定名exclusive完整原件并hash（archived-unchecked）→token/schema预检与资格状态→capture的同类预检→comparator。create预测+chain1+definition1+note最多1；schedule预测+trace1+actual=当前planned数（≤2）+new day最多1+copied note≤1。预测基于已验证before计数，动作前检查两端共同上限；不允许先执行Swift分配children再发现超量。String/hex预算在解码hex或复制文本前，raw预算在JSON materialize前。

cross顺序：原件计数/hash核实→限长读→token/结构→M1 schema→只读decoder。不能先Swift JSONDecoder/JSONSerialization或GoInspect/Decode再补budget；Go Inspect还需明确RequiresClassificationAudit=false才Decode；任一前置失败，后续decoder调用计数必须为0。固定seed也走全链，不因可信而跳过。原件bytes不canonicalize、不排序、不ID替换。

## 5. 状态机与统计

case初态pending；seed前置/真实双端验证全成功→valid；seed资格拒绝→invalid-fixture；未知profile→unsupported；工具/资源/来源错误→harness-error。后三是case终态，命令不执行。valid case命令拒绝时记录rejected，不能effective；映射歧义使对应point not-comparable，不掩盖已存在raw difference。

effective point要求fixture valid、命令（seed除外）成功、完整9原件、所有必要stage执行、bindings完备、七comparison均为equal/different/rejected中的实际结果且无工具失败；cross拒绝可以作为有效观察，但interop不增加。interop point要求effective且七层全部equal。raw随机ID不同就不会interop成功，仍可诚实完成观察。

18case预期共32 point（4 seed-only=4，14 one-command=28）；effective最低必需32，任一未达不得observationAccepted。负例通过只增negativeGatePassCount，不增effective/interop。reportComplete仅指所有18case、M2待办和负例格都有最终状态；它可在零effective时为true，但observationAccepted=false。observationAccepted要求32 effective、所有V负例pass、全证据齐全；interopAccepted另要求32 interop。任何unsupported/invalid-fixture/not-run/ambiguous/harness-error均不能“补足”必跑集合；差异和失败完整保留。

## 6. 精确公共标量与manifest包装

Key=[a-z][a-z0-9-]{0,79}；caseID/commandKey/seedID/NoteID另限64且必须registry成员。UUID=[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}。SHA=[0-9a-f]{64}；SHA40=[0-9a-f]{40}；Bits16=[0-9a-f]{16}；Hex=偶数长度小写十六进制。UInt为JSON无符号十进制整数，Int为signed64十进制，无+、leading zero、小数或指数；Bool仅true/false，String为strict UTF8。空值后缀OrEmpty只允许""，不得null。所有元数据不超过§4累计限制。

SourceManifest-R2.json封闭根={schemaVersion:2,repository:固定官方URL,commit:SHA40,rootTree:SHA40,coreTree:SHA40,storageTree:SHA40,files:[File],toolManifestStatus:固定说明,goCommit:SHA40}。File字段见§4，冻结源恰19项（14 Core+2Storage+Package/LICENSE+App），无tool row；implementation manifest未来另件最多45 tool row，每项同File形状role=tool、gitBlob空或受认证blob，具体path/hash必须在实现候选独审冻结。toolManifestStatus当前not-implemented，不能据此执行。App tree从root→App→NoonmarkMacApp→该blob认证，不把目录响应中URL当签名；Storage tree在此冻结、需对commit验证。root/Core沿原manifest锁值。

FieldSchema附件types语法只含：标量string/bool/int/uint/uuid/stamp、命名objects/aliases、enum:逗号集合、enumint:逗号整数组、[]T数组、pairs:K:V交替数组。M1所有pairs均长度0，未实例化的命名类型只可处于absent optional或empty-only元素位置；其他引用无法解析即profile错误。禁止在缺类型定义时递归跳过字段。

序列化前的保守输出预算：对已验证typed state按schema计nodes（含object keys）、所有UTF8字符串bytes和实际深度，使用checked上界128×nodes+6×stringBytes+2（覆盖pretty indentation、标点、字段、最长整数词法及escaped字符串），若>256KiB，在调用实际serializer前拒绝；此界可保守拒绝，不得调高以填满结果。命令前把create/schedule新增实体完整固定字段及复制note字符串增量加入同一上界，并预留该point最多9件及case总raw额度。真实输出仍逐件复查byte存档预算，保存后再作token/schema资格检查。此为工具有限范围，不修改产品codec容量合同。


## R3存档失败与实现澄清

1. 原件安全存档只依赖固定路径、exclusive-create、单件bytes、件数与全case/run累计bytes预算，不依赖JSON有效性。serializer产出的限内但invalid UTF8/重复key/未知字段/null/profile错误bytes须完整存档，标malformed-raw及typed原因；保存成功不代表schema/domain成功。projection亦遵守同样规则。超byte预算或件数/总量超限，绝不能截断原件后填artifact成功：不添加Observation，相关Stage=rejected/resource-limit或harness-error，受影响Comparison不计effective；仅保存有界{point,side,layer,actualBytes,limitBytes,reason}诊断（≤1KiB/项，≤18项/case，纳入80MiB总额）。actualBytes是已观察的原始计数，不读取越过资源边界的无限流。schema-invalid但raw限内与raw超限必须分开。
2. V06/V07增加限内malformed raw保留原字节、hash与malformed-raw状态的负例；验证下游domain decoder/clone/bridge=0，raw difference仍被记录。另测byte+1只诊断无伪完整原件。证据落盘错误也是harness-error，不能声称已保存。
3. FieldSchema-R2.json也是输入：hash核验、≤64KiB、depth≤8、nodes≤4096、每string≤256B、累计strings≤48KiB、objects恰17种、每类型字段array≤19且总恰117条；aliases≤1、emptyOnlyElementTypes≤16、snapshotGoMapping恰9项。其余字段与键逐字等于冻结附件；任何通用JSON物化前先有界扫描。source manifest与tool附件也不能凭可信跳预检。
4. case≤4MiB是比18×256KiB更严格的合计上限，预留按各件已算保守上界checked求和，不无条件预留18个最大单件。3轮重复的raw、metadata和logs都计入80MiB run上限，不增加首轮32point分母；超限保持拒绝而非加大预算。
5. Stage以(point,side,name)唯一；不适用也须显式not-applicable。每point必有controller preflight；两端preflight/snapshot/encode-package/encode-export/self-decode/self-encode/cross-preflight/cross-decode，Go另from-domain/cross-inspect；seed点两端seed-prepare/seed-decode/seed-core必需且command为not-applicable；命令点command必需，三个seed阶段为not-applicable。Swift from-domain/cross-inspect明确not-applicable。失败依赖后续记not-run，不得缺stage。side在cross阶段表示接收端：go=Swift原包→Go，swift=Go原包→Swift，两个结果分别保存，不能只记录总rejected。每端必须恰14个StageName全部出现（success/rejected/not-run/not-applicable按阶段资格选择），加controller preflight为29项，仍受原≤32上限约束；其余controller阶段不允许，未知stage拒绝。
