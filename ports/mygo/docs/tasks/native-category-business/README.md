# 普通分类 Service-command 冷开候选

## 范围与验证状态

本候选验证单个 Linux/amd64 host 上的 **native ordinary Service-command business cold-open**：真实窗口显示后，在该窗口绑定的同一个 Service/hub/app 上创建一条今日任务并新建、选中一个分类；正常关闭，再以同一版本二进制启动第二个全新进程，正常 Open 同一合成会话并核对持久事实。

业务命令由受控探针调用 Service，不由用户输入触发。窗口内存读视图已接收事实的确认，不等于 UI 分类回调、OS 点击或键入、显示像素、IME、GPU 或全平台验收。默认运行 profile 仍为 legacy-development；所有资料仅限 development/e2e 合成，不导入生产资料，不接入 iCloud、Keychain 或真实 AI 网络服务，不引入 Swift 运行时。

截至 2026-11-07，候选 d6919c0addcf92d40ea6d262ce8eaba5bfbb1fd3 的聚焦 Go 单元与 race 检查、source/history 守卫和 139 项 Python 合同测试已通过。随后完整组合门禁实际失败：scripts/check 耗时 768 秒、退出 1；root 包耗时 475.878 秒，TestRuntimeExitMainHasSingleClosePerPath 报 unexpected Service ownership paths，并非测试超时。root 其余子包随后按 go test 的原行为完成，脚本退出后未继续执行 root vet、其余三模块 race/vet、六目标 cross-build 或 entrypoints。完整门禁与真实 host 业务验收均未通过，不能用先前聚焦结果替代。同进程新 Service 的 Open、注入回调或合成报告均只作单元证据。

本页给出可直接阅读的执行与证据边界。设计版本标识为 SHA256 `c77495c59cd9ac3f529056a60487eb47718ba92dec983fcb2694b2aabe44d9b0`；实现与测试入口见下文，不要求读者访问其他设计文件。

## 固定场景

显式选择 `-native-business-scenario=ordinary-category-v1`，并同时指定 `-runtime-profile=ordinary-development`、`-native-window-smoke`、一个 init/session 动作及 day 页面。driver 对应参数为 `--business-scenario ordinary-category-v1`，仅接受 Linux/amd64。没有任意脚本、业务 payload、日期注入或现有会话导入入口。

1. driver 创建全新私有目录。产品先完成纯参数及路径检查，确认固定 sibling 报告路径均可用，再独占创建报告和会话。真实窗口 ready、visible 且尺寸及 native handle 有效后，才开始业务步骤。
2. 第一进程确认真实 Service 的任务图与分类状态为空，通过既有 writer lane 调用一次 AddToday，标题固定为 `Native smoke task v1`。完整读视图必须恰有一条今日任务，store revision 增加 1。
3. 调用一次 ReplaceCategorySelectionOn/ChoiceNew，新分类名为 `Native smoke category v1`，颜色为 `#D87831`。必须取得真实已确认的 CategoryOutcome 与 receipt；任务图保持不变，最终 store revision 较初始增加 2，classification revision 为 1。AddToday 只返回 error，不为它构造 typed Outcome 或 receipt。
4. 在主线程向同一 hub/app 发布完整读视图，核对实际图、分类、日期、revision、窗口绑定及 runtimeError。只有回调体完成全部确认后才输出 READY 并请求正常关闭；Invalidate 本身不算像素证据。
5. 事件循环返回后停止新业务任务准入，drain 已受理 writer 工作，在 Service.Close 前重新核对已确认的完整事实与 writable Health；再 drain dispatcher、执行真实 Close，并完成生命周期及业务报告写入。任一关闭、清理或报告错误均不能算成功。
6. 第一进程自然退出且清理确认后，driver 才启动第二个新 OS 进程。它正常 Open 同一会话，只验证已有事实，不补种任务、不生成 interaction、领域或 Service plan/confirmation。记录中的 plan ID 是持久数据，不是可复用权限。

## 持久事实与精确断言

PersistentFactsV1 包含完整 Graph 与 Classification；Today 和 StoreRevision 单独严格核对。两个进程的原始 DTO JSON 字节、SHA256、完整对象与整个有界 session 文件清单/hash 必须相同。运行期 Service owner、健康状态及指针不作为跨进程持久事实比较。

真实 Service fixture 已确认：初始 Days 和全部分类集合为空；首次 ChoiceNew 只有一条 setCurrent，RelationHistory 为空，分类、current relation、change record 和 receipt 各一，capture 集合为空。Changes.Before.Labels 与 After.Labels 的 DTO 值精确为 null；持久 receipt.Notices 为 []，实际命令返回的 receipt.Notices 为 null。这些形态分别验证，不归一化，也不同时接受两种形态。

core.Instant 的 JSON 是 IEEE-754 位模式对应的 uint64 整数，不是秒数。原 DTO 字节不经浮点或重序列化转换；时间先后按解码后的有限 Seconds 比较。Go 生成的 3503 字节单元样本已通过 Python 独立全字段及 PlanDigest/IntegrityDigest 重算。冷开验证只重算既有固定 ASCII digest 字节，不调用 PrepareAggregatePlan。

完整 DTO 只代表 Go 支持的 ordinary 事实投影，不代表完整 v7 wire codec 或 Swift 实体互通。磁盘清单一致只证明两个取样时点的持久结果一致，不排除未观察的中间写回，也不是硬断电耐久证明。

## 日期、取消与失败资料

产品在 Create/Open 紧前、返回后、每个业务命令前后、发布确认和最终报告阶段采样实际本地日期、时区及 offset。日期或 offset 变化均按 date-boundary-not-covered 失败，不回拨时钟、不重建会话、不自动重试。

正常 Open 可能先执行自动日结并写入，再返回 Service；跨日失败不代表零写。取消或关闭也不能撤销已经受理或提交的命令：关闭准入只阻止后续提交，已执行的写入可能保留。丢弃或迟到的主线程回调不能形成成功确认。

失败后保留本轮私有目录及报告供检查。第二份报告独占创建失败时，第一份空报告也可能已存在；Create/Open 之后的错误可能伴随写入。结果不确定时先检查现有资料，不在原目录重新执行 create 求绿。证据上传仅使用明确 allowlist，不上传 session、HOME、cache、cookie 或环境资料。

## 预算、源码与后续验收

保留既有软件 watchdog：总计 120 秒、每阶段 60 秒、READY 45 秒、完成 55 秒、READY marker 后 10 秒、close request 至 finalized 10 秒，末段清理最多 5 秒。业务使用原有准备预算，不增加额外时间。每份报告上限 64 KiB，日志上限 1 MiB；不可中断的 OS 调用仍可能超出软件控制，缺少完整清理证据时必须失败或标为未知。

product v3 精确绑定原 120 项到新 122 项产品文件及 6 项 delta；原 v1/v2 常量和 73 项历史 overlay 链保留。脚本、工作流及依赖准备链另有 38 项完整守卫，拒绝未声明的增删、字节或路径变化。旧六 host 默认 schema、allowlist 和 workflow 原字节不变。

[独立业务 workflow](../../../../../.github/workflows/native-category-business.yml) 使用现有正常 scripts/build 产物及 smoke.pack，仅运行官方 Ubuntu 24.04/Linux amd64。二进制来源、commit、run/build/consumer attempt、artifact ID 和实际 executable hash 必须一致，collector 与 gate 分别核验；上传成功不能掩盖业务或清理失败。产品仍需通过原跨平台组合门禁，不用本场景替代六 target cross-build。

后续应在同一待交付版本上完成四模块 check/vet/race、entrypoints、source-freeze、旧新 native fixtures 和 cross-build，再审查真实 host 的二进制、两个独立启动实例、两个窗口、完整原始报告、持久事实、磁盘清单及受管进程清理。所有条件成立才可记录本场景通过；其余平台、UI callback、OS 输入、像素、IME/DPI、多窗口和崩溃恢复仍需分别验收。

实现入口：[业务控制器](../../../native_business_probe.go)、[完整事实断言](../../../native_business_facts.go)、[driver/collector 使用及证据边界](../../../scripts/native-window-smoke/README.md)。构建及测试须使用现有 scripts/environment 准备的固定 MyGo 依赖，并将 build、cache 和日志放在外置 NOONMARK_MYGO_BUILD_ROOT。

早期未加载 prepared dependency 的编译失败、fixture 的 null/[] 预期错误，以及测试等待 callback 导致的 120 秒超时均保留为失败记录。修正使用既有依赖准备流程、精确 DTO 形态及测试内 5 秒 fail-fast 等待；相关单例与聚焦检查已回归通过，没有放宽产品或 host watchdog。

此次完整门禁失败暴露了启动清理源码合同的覆盖缺口：旧合同限定三条 Service.Close 路径，新业务路径在正常 Open 成功后、after_open 日期采样失败时另有一次 Close，随后将日期和 Close 错误一起交给 log.Fatal 终止进程。文本计数不符本身不证明运行时重复 Close，也不能仅把计数改为四。修复需明确这条早期路径与 render、分类初始化失败和正常退出三条路径的互斥关系，保留旧错误传播与退出顺序断言，补充完整 main 的有限源码合同、窄 AST 及故障负例，再以同版本重新完成聚焦和完整门禁。清理合同修复已实施并通过相关聚焦检查；完整组合门禁仍待重新验证。原失败证据保留。
