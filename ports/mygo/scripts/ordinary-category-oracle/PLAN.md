# 普通单分组 Swift 独立参考器：设计范围

本参考器只用于合成普通分类的独立研发对照，Go + MyGo 仍为唯一产品。实现与现有 name/snapshot suites 完全分开，不读取其运行门禁作为许可，不修改 fullI1/runtimeAdmission=false、1807 分母或固定输入。Python/static/mock 检查不等于 Swift 执行证据；本次尚无实际 Swift 捕获及 Go 对照。

## 精确源码闭包

SOURCE-CLOSURE.json 固定 quboliu/noonmark@43e6eefaae3a04be89cab3e234d470111afd80ef 的全部 14 个 NoonmarkCore Swift 文件，原 bytes/SHA256/Git blob，不裁切、不 patch、不替换 UUID 或 Date 实现。上游 Core 为互相引用的 extension 模块，完整编译这 14 文件避免伪造依赖类型；调用范围仍仅普通分组。另保留 AGPL-3.0 LICENSE。闭包列出公开仓库、固定 commit、Git tree、每文件公开 URL 及本目录内相对路径，可独立复核。

compiled：AutomaticTaskClassification.swift、IdeaWorkspace.swift、Identifiers.swift、LocalDate.swift、Models.swift、MutationClock.swift、NoonmarkEngine.swift、NoonmarkError.swift、NoonmarkSnapshot.swift、TaskClassification.swift、TaskClassificationStateValidation.swift、TaskCycleSeries.swift、TaskDefinitionValidator.swift、TrajectoryTopologyValidator.swift。

全部 14 个 Core 文件及 LICENSE 的原始字节已保存在 upstream/，SOURCE-VERIFICATION.json 记录逐文件 bytes/SHA256/Git blob 核验结果。runner 编译前再次核验完整闭包；缺件、额外文件、链接或身份不符即拒绝，不下载替代来源。

不编译 Storage、App、测试套件；App/TaskClassificationEditor.swift:461–478 与 Store+ClassificationReview.swift:336–366 只作已核行为依据。不会创建/启动 App 或调用 iCloud、Keychain、AI、生产存储。编码对象是 TaskClassificationState 和 TaskCategory 的真实 Codable，不调用 NoonmarkDataPackage、完整 snapshot encode/decode 或任何 admission 路径。因此不构成 fullSnapshot 封闸的替代路线。

## 局部模块与真实 API

一个 OrdinaryCategoryOracle.swift 可执行文件，@testable import NoonmarkCore；Core 用 -enable-testing 编译。通过实际 createPoolTask 生成两条合成 ordinary chain，再调用 prepareClassification(.setCurrent)、ClassificationConfirmation.user(confirming:decisionID:)、commitClassification、TaskClassificationState.validateIntegrity、JSONEncoder.encode(state)、JSONDecoder.decode(TaskClassificationState.self)。不增加 callback validator，不改变上游访问控制、私有摘要函数或随机实现。

私有 Encodable material 不可访问，故镜像精确字段/类型/optional omission 的两个本地 Encodable 结构。数据全部从实际返回 plan 与 record 读取；真实 UUID 值及 UInt64 日期位不改动。只在 SHA256(reconstructedPayloadBytes)==实际 plan.digest / record.integrityDigest 且原 hasValidDigest/hasValidIntegrityDigest 都通过后将该重建收据标有效。始终标为 reconstructedPayloadBytes，不声称截获上游私有 encoder bytes。普通子集 plannedLabelIDs=[]，非空立即失败。

## 固定小输入与阶段

输入只允许一份冻结 manifest（<=16 KiB）中的 suite ID、版本、两个合成标题、交互 ID、确定性 reference-date UInt64 bits、10 个固定名称向量。整文件 exact bytes/hash 固定；不是通用 JSON command executor。命令及 mutation 列表写在 harness，不接受任意路径、source、任务动作或可执行代码。运行必须显式选择 capture 并提供已审工具清单 SHA256，默认仅 verify。

主序列 A：new 工程 → 新 interaction 再传相同 .new 工程 → 原始首个 plan 精确重试（原 now）→ 同 plan 重试（不同 now）→ none → none。B：none → existing 工程 → new 工程重名复用。decisionID=interactionID。记录每次 prepare 输入、prepared intent、plan、确认绑定、receipt、全 classification state、状态/审计数量、空 current 键是否存在、逻辑／日期位不变检查及因果随机 ID 槽；原始字节另外完整保留，不能由逻辑相等推断字节相等。

负例独立分支从真实正控制复制：两个同 base plan（第一个成功、第二个 stale）；已提交 plan 改 decision；原 plan 改 preview 保留 digest；改 preview 并重算有效 digest（应由 recompute changes 拦截）；改 digest；缺 removal、origin decision、removal 时间逆序；相关 relation 时钟逆序；另一 chain 较早时间；record canonical order 变化。每一项分开记录 prepare/commit/integrity/encode/decode 的实际 outcome 与 error type/message，绝不把下游 decode 失败改写为 commit 失败。正控制同记录，不先假定某个 mutation 必失败。

名称入口：direct TaskCategory initializer、prepared standalone create（仅参考）、core setCurrent new、UI reference input（先调用一次 D，再 core；不执行/验收 SwiftUI）。每个 raw UTF-8 用 base64 保留，记录 raw、D1…D5、每一层 K、实际 prepared 名字、preview、最终 stored name/key。含空白+combining 非幂等、中文、组合音、全宽、ß、土耳其 I、default ignorable、斜线/quote/control/U+2028/U+2029 和长 nonstarter。raw 非幂等名字绝不预先清洗。初始 category/record dates 使用确定性 supplied clock，含负零/亚毫秒独立分支。

## 小输出契约

输出目录由外层创建且必须为空。每个观察有一个 bounded JSON 文件，schema=ordinary-category-observation-v1，含 caseID、stage、status、originalIDs、clockBitsHex、input、plan/record digest references、raw classification/category bytes references、canonicalizer algorithm version、少量 typed fields。独立 manifest 列出路径、bytes、SHA256、角色。原始 Foundation payload/state/category/record bytes 以独占创建原文件保存，不以 JSONSerialization 重编码代替。additional explicit date bit projection 保留原 Date 所有位；默认 Foundation Date Codable 输出/读回若失位，记录原位和回读位及阶段。

随机 ID 槽仅观察实际链、definition、plan、planned category、NameVersion、record、removal UUID。不固定或注入上游 UUID generator。无跨运行 UUID 替换，无 digest 对归一化 bytes 的错误比较。后续 Go 对照可在保留并复验原件后另建 typed 因果映射，本生产器不宣称 Go 对等。

每个原件 <=256 KiB，最多512件，总原件 <=16 MiB，单观察 JSON <=64 KiB；输出清单 <=128 KiB。超过上限立即 fail，保存有界 failure.json 与已经完整落地原件，不保存截断伪原件。命令层 stdout/stderr 各 <=64 KiB；总运行 deadline60s、编译 deadline900s；无无限重试。任何异常退出、来源不符、摘要复算不符、输出不完整、未运行都不能生成 verified=true。预期领域拒绝是实际 observation，不自动等于整个 suite 失败；准确收集与业务对账结论分开。

## runner 和 CI 边界

runner 使用固定来源／工具清单、独立 HOME/TMP/cache/build/run、macOS + Xcode26.2 / Swift6、原 Core -enable-testing 及 deny-default/deny-network sandbox-exec。输入仅冻结 INPUT；先封存并核验来源，再编译／运行，每阶段和最终 receipt 前复核身份。实际 Xcode 可用性与 sandbox 兼容性须由真实运行确认，失败不改选、不下载工具链或放宽策略。

仓库的 .github/workflows/ordinary-category-oracle.yml 采用 contents:read、固定 action SHA、不保存 checkout 凭据、macos-15、cancel-in-progress:false，仅 main 的参考器／CI辅助目录／本 workflow 专属路径 push 和 workflow_dispatch 触发。无 PR、secrets、签名、发布或 production。上传仅限封闭的合成证据清单；执行、上传及失败边界见相邻 ../ordinary-category-oracle-ci/README.md。

真实 Swift 输出尚未取得。不会写 golden、拿 Python/Go 自测冒充 Swift，或提升产品准入。
