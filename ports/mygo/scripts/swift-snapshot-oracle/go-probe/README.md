# 独立 Go 快照 probe

仅供冻结 R3 M1 合成表示观察，不是产品 runtime、迁移器或互通验收器。所有新 Go 源码为 AGPL-3.0-only；产品及既有 name oracle 未更改。

从 `ports/mygo` 使用指定官方 Go 1.27.3：

    go build -o "$NOONMARK_MYGO_BUILD_ROOT/go-snapshot-probe" ./scripts/swift-snapshot-oracle/go-probe
    SNAPSHOT_SPEC="$PWD/scripts/swift-snapshot-oracle/spec" go test -count=1 -v ./scripts/swift-snapshot-oracle/go-probe

运行接口：

    go-snapshot-probe --spec SPEC --fixture SEED --case CASE_JSON --vectors VECTORS --output NEW_DIRECTORY
    go-snapshot-probe --spec SPEC --cross ORIGINAL_PACKAGE --output NEW_DIRECTORY

`CASE_JSON` 必须结构上逐值等于冻结 registry 中一条 case，不能自带新动作。seed/vectors/schema/registry 具有编入 probe 的固定 SHA256 核验。源文件本身的 commit/tree 认证、进程沙盒、超时、stdout/stderr 有界读取、全运行及跨两端证据总预算由外层 runner/controller 执行；不能只运行 probe 就声称这些门禁通过。

每 case 的 `result.json` 与 stdout 相同，含 schemaVersion、caseID、fixtureValidity、points、interopAccepted=false。point 含 key、returned、stages、artifacts 和 selfEqual。`returned` 为成功命令后核验存在且唯一的显式 Go 命令 ID；Go API 本身只返回 error，不伪称它返回实体。seed 无 returned ID。直接 Go 字段投影由冻结 schema 逐字段读取，独立于 FromDomain；日期读取 Instant 的 UInt64 bits。五个 Go 原件均为新建固定路径：kernel-projection、from-domain-projection、package、export、self-reencoded。

实际流程使用 canonical.Parse → v7.ToDomain → core.NewTaskKernel → core.CreatePoolTask/ScheduleFromPool → v7.FromDomain →真实 v7.Codec.Encode/Inspect/Decode。nil classification audit validator 只服务冻结零 history 范围，没有 fake validator 或 taskapp adapter。export 在独立 FromDomain 副本仅关闭 sync enabled。self Decode 后重新编码前另核完整 decoded snapshot 与编码前 snapshot。所有原字节保留，不在 cross 前标准化。

cross 结果在新目录 `result.json`，成功时另写 `decoded.json`。失败 exit 2；accepted、status、inspectCalls、decodeCalls 共同判读：preflight 失败时两计数为0，此时 requiresClassificationAudit=false 只是未发现，不是执行了 Inspect 的证明。父级不能把无 decoder 调用当作接受。

合同测试覆盖实际全部18个 Go case/32个点、直接投影与 FromDomain、真实 self roundtrip；另含限额、重复 key、surrogate/UTF8、unsupported profile、原件先存档、exclusive-create、cross 原包与加空白拒绝顺序和 decoder 调用数。它们不是全部 V01–V09 验收，也不是 Swift 运行证据。Swift 真实结果仅能由经审查授权的 macOS runner 获得；M2、R/X 均未运行。

固定资源负例接口：

    go-snapshot-probe --spec SPEC --mutation-id raw-depth --output NEW_DIRECTORY

24个 ID 在 mutation.go 封闭列举，与 Swift probe 相同。负例 bytes/state 只由工具内部生成，不接受 payload 或自定义限制；unknown ID 拒绝。result.json 包含实际 accepted、reason 和五类下游 operation-entry 计数。计数点位于正常捕获也使用的 instrument.go 包装入口；它们证明该边界没有进入相关调用，不声称追踪 codec 内部每一次分配/clone。mutation 执行前先验证固定 empty seed，随后清零该次负例操作计数；不是把先前 fixture 校验冒充负例操作。

raw 七项覆盖 byte/depth/node/单string/累计string/array/member。profile 五项覆盖 empty-only、奇数/重复/nonempty pairs、Base64未知字段。hex 四项覆盖奇数、非法UTF8、decoded上限+1、两倍扩张。create/schedule 各四项覆盖 nodes/entities/text/保守输出增长预算。预测 fixture 是工具专用负例，不纳入18case/32point分母，不证明新业务 fixture 合法。

## R2 实际入口矩阵

    go-snapshot-probe --ingress-matrix normal --spec SPEC --output NEW_DIRECTORY
    go-snapshot-probe --ingress-matrix bypass --spec SPEC --output NEW_DIRECTORY
    go-snapshot-probe --ingress-matrix late --spec SPEC --output NEW_DIRECTORY

矩阵通过普通 loadFrozen、matchCase、runCase、archive.observe 和 cross 路径调度，而非把入口名字贴在另一份 parser 单测上。四个 seed 分别执行其真实 observe case；五个 Go 输出入口在普通原件落盘后使用具名 shared preflight。配置也经过普通 authenticated loadFrozen。固定恶意 bytes 在选定入口进入共享预检时注入；没有调用方 payload 或预算覆盖参数。普通路径本身使用相同 parseIngress 和真实操作包装器。

每个入口同时有正控制与预算/类型负例。事件从该入口首次进入时开始记录，因此不把之前合法 fixture setup 的调用误算成被拒入口的“下游”。materialize 是实际 encoding/json Decoder.Decode，decode 是真实 ToDomain/Codec.Decode；clone、bridge、command、serializer 是 instrument.go 的实际操作入口。schema 负例允许预算通过后的 JSON materialize，但不允许域操作。scanner 负例不允许 materialize。正常 create 与 schedule 分别独立执行，并要求五类真实操作都出现。

bypass/late 会改变该实际 shared preflight 与实际 materialize 的先后，而不是直接返回一个预设失败结论。bypass 跳过 scanner，late 先 materialize 后 scanner；矩阵根据真实事件、拒绝、调用计数复算 pass。两种 fault 模式的进程正常退出，但矩阵应为 false。正常矩阵294行，任何缺行/重复/未达入口或不合事件序均不可合格。这个结果不改变18case/32point，也不证明 Swift 已编译。

某些固定预算重叠，特别 vectors 累计字符串限制大于或等于16KiB raw文件限制，无法在不先撞 raw 限制的前提下到达累计字符串超限。相应行的输入确实超过目标限制，但实际先行拒绝保持原始 reason；不得宣称后面的 guard 独立执行过。独立每限制单测另行保留，不能用它替代实际入口证据。

每行 inputBytes/inputSHA256 绑定到具名预检实际收到的原件或固定注入 bytes。注入只用于资格检查，不能把原先保存的有效 serializer 原件说成这些恶意 bytes 的存档。create/schedule 正控制绑定普通 matchCase 实际核验的 case JSON；不计业务点。
