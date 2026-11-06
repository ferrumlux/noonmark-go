# 手动进度验证记录

验证日期：2026-11-05。Task ID：connect-manual-progress。源语义基线：`43e6eefaae3a04be89cab3e234d470111afd80ef`。候选从研发工作树复制，起点HEAD为`ee6e484`；复制时该树已有待提交的Unicode16验证文件，此批不交付、不修改它们。只合入本文列出的文件，不用整棵副本覆盖并行UI／分类改动。

## 已验证功能

新增18个顶层测试（core 9、Service/audit 9），focused race输出合计65个通过事件（包含顶层与子案例），0 fail、0 skip。测试的合法领域图均由实际CreatePoolTask、ScheduleFromPool、SetManualProgress、completion、return、subtask、settlement命令生成；审计负例只在合法候选的独立clone上故意篡改。

- 查找、chain+trace owner、invalid LocalDate、locked/past/future/non-pending错误次序；错误不发布。
- clamp涵盖MinInt/MaxInt，nil→0必须写入，100不完成；允许当前数值下降至历史floor。
- 同值clamp/floor结果no-op保留所有字节和时钟，核心不检查未使用的NaN/Inf/倒退now；变化接受相等有限clock，只比较目标content clock，拒绝倒退/非有限。
- 历史／同日／未来的其他trace实际child均阻断；当前trace无child并非充分条件。cancelledDraft child与取消父trace的child不计；历史definition中planned child不计；其他chain的child不阻断。
- 完成→撤销保留manual值；回池→跨日排期适用历史floor；同日同sequence不自行产生floor；原版continuation命令尚未移植，本批不声称新增该路径。
- Service create→set→reopen精确图相等、nil/0和IEEE clock bits编解码不丢失、Snapshot指针无别名。
- no-op不采样wall clock、不新增revision，但仍拒绝stale prefix、损坏COMMITTED、blocked及closed句柄。
- 四阶段before-reserve／after-reserve／before-commit／after-persist故障均不提前发布或返回成功ack；前者重开原状态，中间两者拒绝重开，after-persist重开保留精确已落盘值。
- 同句柄6个并发写入各自取得本次确认值；跨句柄竞争只准一个成功，失败者不发布。倒退wall clock仍由全图单调frontier提供写入时间。
- 精确命令重放拒绝percent、clock、status、priority、description、其他trace、chain、definition、day及删除trace篡改，拒绝错误命令和不匹配before。
- 非空catalog、缺失历史、无效manual范围继续fail-closed，完整分类历史不重写；没有放宽任何validator、codec或store入口。

初次新测试将损坏COMMITTED预期写成ErrIncomplete，实际既有verifyPrefix明确返回ErrCorrupt；纠正了测试预期，未修改既有实现。此结论仅属于初次作者验证；独立审查后来发现既有极值重开缺陷，修复及新增故障案例 GO-RUNTIME-013 见下文。新增功能测试由现有scripts/check的全包测试自然纳入永久验证。

## 执行命令与证据

工作目录为`ports/mygo`。所有新证据、二进制和临时资料位于显式外置`NOONMARK_MYGO_BUILD_ROOT`。工具链Go 1.27.3，GOPATH与GOCACHE均外置；没有读取production、iCloud、Keychain或调用AI，没有远端写入。

- `scripts/check`：退出0；四个模块gofmt、全包race及vet通过，Unicode16生成／路径／manifest门禁通过，51项dependency预备器测试通过，外置MyGo native边界race+vet通过。原有测试未删除、修改或skip。
- `go test -p=1 -race -buildvcs=false ./internal/taskapp -run '^TestManualProgress' -count=1 -json`：通过，43个pass事件。
- 在domain-core内执行同参数`./core`：通过，22个pass事件。
- `go test -p=1 -buildvcs=false ./internal/taskapp -run '^$' -bench '^BenchmarkManualProgressServiceYear$' -benchtime=1x -count=1`：通过。
- 对continuous-20额外采CPU与alloc_space profile：通过；测试二进制通过`-o`显式写往外置bin。

外置证据文件名：`full-check.log`、`manual-core-focused.log`、`manual-service-focused.log`、`manual-performance.log`、`manual-profile.log`、`code-sha256.txt`；`evidence/`下有两个focused JSONL、CPU/memory pprof及top/cumulative文本。

## 性能发现：尚不具备滑块写入体验对等

fixture为365天各1个真实领域命令生成的普通任务，364个过去日已结算，包含completed与unfinished历史及完整empty catalog audit；当天任务可编辑。序列化快照895,504 bytes。fixture通过一次测试专用aggregate提交安装，不是公开导入器，也不是上游完整一年demo故事。

Linux/amd64、AMD EPYC 9V74、GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB，本机单次样本：

- 一次变化：1.998秒，638,254,392 B allocations。
- 连续20次变化：47.965秒，平均2.398秒／次，累计17,874,407,704 B allocations。
- 连续20次同值no-op：28.334秒，平均1.417秒／次，累计9,654,155,056 B allocations。
- 另一次profile连续20次变化：46.435秒。数值是当前共享执行环境的实测，不是P95、跨平台指标或内存常驻峰值。

CPU profile共77.17秒samples，含fixture setup与后台GC；累计百分比相互重叠不能相加。GC scanObject累计37.10%，ValidateEmptyHistory25.71%，normalizeSnapshot20.81%，captureManualProgressHistory13.04%，disk.verifyPrefix11.84%。NewProgressProjection累计仅0.12秒/0.16%，分配65.73MB/0.36%；在此fixture里并非主要瓶颈。allocation大头为v7.clone22.30%、JSON decoder fetch19.50%、bytes.growSlice13.11%。下一步应围绕共享codec／审计重复完整图转换、clone与revision prefix扫描做根本设计，不得为了性能绕过完整校验、审计或损坏检测。

上游TaskDetail事件经Store+TaskMutations:1674同步commitEngineMutation，后者用changedFrom原快照差量持久化。本批只提供Core/Service整数命令，不接slider，不将“明确提交”、拖动结束写入或丢弃中间事件当成已获授权的对等替代。开发store仍有1024 revisions／24MiB单条／128MiB会话限制；现有append-only JSON开发后端不等于SQLite正式持久化。性能是实际未达标项，不能宣称手动滑块体验完成。

## 交付文件与边界

- `domain-core/core/manual_progress.go`
- `domain-core/core/manual_progress_test.go`
- `domain-core/core/progress.go`：仅把能力查询注释更新为指向新命令，无算法变化。
- `internal/taskapp/manual_progress.go`
- `internal/taskapp/manual_progress_test.go`
- `internal/taskapp/manual_progress_audit_test.go`
- `internal/taskapp/manual_progress_benchmark_test.go`
- `internal/taskapp/README.md`：仅追加本批能力、事务及性能边界。
- `docs/tasks/connect-manual-progress/DESIGN.md`
- `docs/tasks/connect-manual-progress/VERIFY.md`

没有修改旧测试、UI、MyGo、reference或正式树。旧 service.go 的日结时钟与 no-op 安全修复是本批明确扩大范围。未运行Swift oracle、macOS/Windows实机或断电测试，不涉及完整undo/snapshot-undo、SQLite、非空classification、recurring、飞光与生产同步。功能验证和独立复审不代表发行；当前未提交、推送、发布。

## 独立审查后的极值重开安全补充

新增既有故障修复 GO-RUNTIME-013：合法有限 MaxFloat64 的真实 manual 命令落盘后，同日无变化 Open 原先错误推进 nextInstant。接手副本原红断言再次失败后，改为共享 persistedClocks／只读 frontier 预检；只有确有变化才从原图以严格递增时钟重放完整结算与审计。no-op 不采样 wall clock，仍拒绝 closed、blocked、旧 prefix 损坏及冲突；AdvanceTo 同一天不再跳过事实检查。没有放宽 kernel、validator、codec 或历史审计。最初故障引入 commit 为 4cbd56883b8a6065c82902923b66ecc0741a2cef；501fdfca858a6c437dca4875aaca4bd260627fbf 保留同一错误顺序。完整归因见案例。

扩大交付范围，在上列10文件之外增加：
- internal/taskapp/service.go
- internal/taskapp/reconcile_clock_test.go
- internal/taskapp/exhausted_frontier_reopen_review_test.go
- internal/taskapp/manual_progress_independent_review_test.go
- docs/engineering/failure-cases/GO-RUNTIME-013.md
- docs/engineering/failure-cases/gates.tsv，仅追加 GO-RUNTIME-013 一行，不覆盖后续正式树其他登记。

原红例文件 SHA256 e4b9d91965e362d0cc18c4fb649ec12b6cf5b5d00b1ba042cd56be1db5dea9df 与独立5项测试 SHA256 6aad143089eff31fb4bacecf0a3f87d7bcba77c959277534390ed67030e95d59 均原样保留。新矩阵覆盖常规／极值重开及真实跨日、同一天尚未结算历史、空图／未来图无变化、反向 wall clock、closed／blocked／旧payload／旧seal／身份损坏／跨handle冲突及四阶段提交失败；失败不发布任务、审计或 today。原7代码冻结未改。

外置 reopen-safety/red.log 保存接手红例；focused.log 保存首次新矩阵的错误预期，Open 的已有扫描错误返回 ErrIncomplete，而 live prefix 返回 ErrCorrupt，仅纠正新增断言。focused-final.jsonl 与 core-focused.jsonl 为修复 race×3：28个顶层测试共84次顶层pass，含子案例339次pass，0 fail／skip。其后只去除 AdvanceTo 外层重复 closed guard，语义守卫统一留在 reconcileDayLocked；最终聚合与六编译使用去重后的冻结代码。

最终代码门禁（外置 reopen-safety/）：final-check.exit=0，四模块全包 race／vet、Unicode16离线与路径／manifest验证、51项dependency准备器测试及外置MyGo native边界均通过。final-cross-build.exit=0，linux/windows/darwin 各 amd64/arm64 六编译通过，仅编译不代表实机。final-cli-list.exit=0，Linux输出20屏；final-cli-guard-expected-1.exit=1，缺少隔离运行身份按预期拒绝。safety-code-sha256.txt 固定安全实现和测试；deliverable-sha256.txt、owned-files.txt 为扩展16文件交付清单。正式树检查保持 3c461d5db0afd49ded19839c46c40a5f6bbdac89 clean；候选无Git提交，故障案例保持处理中，待独立复审与整合commit回填。不推送或发行。此前一年容量与性能未重跑，不引用本安全修复证明性能改善；slider仍未接。

## 正式副本整合验证

在正式隔离副本 3c461d5 基线上选择性整合16份作者交付（gates.tsv仅追加本案登记），另加入独立 reconcile_independent_review_test.go 两项永久回归，未覆盖并行分类或UI文件。独立原红例及原5项独立测试字节保持不变；新增独立测试 SHA256 为 0c581bab1f85d32c7fc19d60fe62a79541edd1bc27ef5edc7276a21b26ae9039。原来的“正式树保持clean”仅是交付时观察，不描述整合中的状态。

2026-11-05 正式副本真实执行：scripts/check退出0，主模块race140.881秒、taskapp34.092秒，四模块race／vet、Unicode16离线门禁、51项dependency测试与native边界均通过。scripts/cross-build六目标全部通过；scripts/test-entrypoints在全新隔离开发会话完成create、reopen、day/pool实际MyGo渲染、拒绝覆盖及缺少身份拒绝，退出0。外置日志为reopen-safety/integrated-check.log、integrated-cross.log、integrated-cli.log。所有测试完成后才补本文，无测试期间修改代码。

这是本地源码整合验证，不是GitHub CI、Swift互通、六平台实机或可发行证明。远端main仍为e723b73，待批写入没有重试；一年数据性能限制与slider未接通状态不变。
