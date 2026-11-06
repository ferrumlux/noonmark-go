# 一年中段 carryover 验证记录

task-id: verify-carryover-year

## 验证对象与隔离

本批只新增两个测试文件、一个独立证据脚本及本目录文档；现有业务文件零改动。源码来自正式 462d6da 的 R7 冻结候选，不是已合并、已发布或完整 Swift demo 对等的声明。

运行日期：2026-11-06 UTC。测试固定 today=2026-11-05；365 个日期从 2026-05-07 至 2027-05-06，today 前后各 182 日。最终初始资料为 762 个普通无子任务任务：183 completed、182 unfinished、397 pending，182 个历史锁日。today 共 34 行，包含 32 个额外长滚动 pending 任务。

fixture 由真实 TaskKernel 命令合成，经 CaptureEmptyHistory、normalizeSnapshot 和 ValidateEmptyHistory 后以 test-only aggregate 安装。初始快照 1,393,596 字节、revision=2（初始空库一次、测试安装一次）。不是正式导入器，不手改磁盘 JSON，不绕过验证。

只使用隔离 development session，marker 为 development-v2。没有读取或操作 production、iCloud、Keychain、真实 AI。无 DISPLAY、无 Swift，所有图片来自 headless native Tester 原图，不是 OS 截图或原版截图。

## 已执行结果

- `01-fixture-first.log`：早期 730 任务、每日两项版本的真实生成/验证/Close/Open 通过，11.10s；这是演进记录，不冒充最终 762 任务版本。
- `02-native-first.log`：首次 UI 测试编译失败，只涉及新增测试的 unused import 与错误字段名称。已保留日志；未修改业务。独立静态审查另纠正未执行的测试误解：Deferred 不使用 ChangedToTraceID，关联在新 trace 的 CarriedFromTraceID。
- `03-native-second.log`：730 任务 native+Service 普通运行通过，43.05s。
- `04-native-long-first.log`：762 任务长列表普通运行通过，61.05s。Service Defer 2.988s、Continue 2.972s；加上各自 AdvanceTo 和 UI publication 的观察耗时为 5.187s、5.514s。这是本机单轮观察，不是性能阈值、基准承诺或实机体验证明。
- `05-native-final-race.log`：最终主 native+Service 测试带 `-race -count=1` 通过，183.53s。该轮内部 fixture 子进程为常规 Go test，11.22s；不把父进程 race 标志算到子进程。
- `06-source-isolation.log`：R7 原冻结目录及本独立副本的 35 个冻结文件全部通过 SHA；逐文件比较 existing files 无差异。新增脚本 `bash -n` 通过。
- `07-full-check.log`：完整 `scripts/check` exit 0。root 包 race 通过 461.529s，internal/taskapp（包含 fixture 自检）race 通过 102.658s；四模块 race/vet、Unicode16/path guards、51 项依赖测试、MyGo targeted native 与 popup completion 全部通过。没有抬高默认 root 的 10 分钟超时。未另跑完整应用六目标 cross-build 或 CLI entrypoints，不把脚本内部 Canonical16 pure-Go 六目标编译冒充整个应用验收。
- `11-final-ordinary.log`：最终源码普通运行再次通过，52.85s；不设置任何 evidence/session 输入，只从全新临时目录生成。该次与完整门禁尾段并行，不作为独立性能 benchmark。

## 已确认的实际路径

1. 默认 Day 首次展示今日 34 行。目标末行初始在 Y=1603、无可命中可见面积，实际 RightClick 拒绝；经 Tester.Scroll 后在 Y=660/H=44，可真实点击/右键。固定日期条的 Rect 保持不变。
2. 从该行原生菜单打开延期日期组件，选择 2026-11-07。取消不改 graph 或磁盘。再次打开及确认，pending 期间连续 Return 不增加 writer job。
3. 使用真正 coordinator.SerialLane 单 worker 调用 Service。每次 carryover 前独立一次 AdvanceTo(2026-11-05)，然后 Defer 或 Continue 各恰好一次。test gate 仅暂缓执行以验证排队期间的重复确认，不伪造 ack 或业务结果。
4. Defer 返回不同的新 ID，目标日已有两项任务仍保留，UI 日期及 selection 对应磁盘确认 ID；旧源保留为 deferred，原内容/身份除合法结算字段不变。已有历史审计、无关 chain/definition/trace 和所有既存 Day 保持。
5. 使用前后日及回到今天等真实按钮导航到历史 2026-11-04，completed 不提供 carryover，unfinished 只有 Continue。详情为静态只读，尝试输入不产生写入；未来普通任务不提供 Defer/Continue。
6. 历史未完成源 Continue 到 today，原 trace/历史日/完整 classification audit 不变；新增 ID 同 chain、正确 CarriedFromTraceID、continuationSeq+1。完成后原源不再提供重复 Continue。
7. 两次操作合计 trace=762→764，Day 始终 365，revision=2→4。额外 Frame、过去/未来导航、应用 Close 屏障/detach、Service.Close/Open 均无重放，完整 graph 和验证后的快照/审计重开相等。Close 调用实际 `beforeClose` 生命周期 hook 并验证 owner 释放，不是 OS 窗口按钮事件。

## 原始图片的实际像素检查

最终图片位于外置 BUILD 的 `evidence/carryover-year-srlOch/native/`，全部为 2400×1536 原始 PNG（1200×768 DIP、2×）。八张均已实际查看像素，没有拼接、重绘或伪造 Swift reference：

- `year-00-long-list-top-2x.png`：今日长列表顶部、33 pending / 1 completed、固定日期条。
- `year-01-today-before-2x.png`：真实滚动至末端，20–32 行与完成项可见，32 被选择。
- `year-02-defer-picker-2x.png`：同一滚动背景上的居中延期弹层，选中 7 日且摘要一致。
- `year-03-deferred-future-2x.png`：未来 11 月 7 日，原两项及新延期项并列，右栏日期与选择一致。
- `year-04-history-readonly-2x.png`：11 月 4 日“历史已锁定”，完成与未完成源、静态详情。
- `year-05-continue-picker-2x.png`：历史源的延续弹层，今天快捷键选择 5 日，摘要一致。
- `year-06-continued-today-2x.png`：成功回到 today，右栏显示延续后的任务与 today 日期；列表留在顶部，新行在视口以下。本批没有“自动滚动到新行”的断言，不能据此宣称该体验已验证。
- `year-07-reopened-future-2x.png`：Close/Open 后经真实日期与行操作重新选择延期 ID，未来既有项与新项仍在。

早期 `native-first` 和 `native-long-first` 原图另行保留，部分日期圆处于展示动画中。最终截图只推进 Tester 展示时钟 200ms 后保存原渲染像素，不推进 Service today 或改事实。

关于新选中行的滚动，只读复核原版 43e6eef：`App/NoonmarkMacApp/DayTodoPage.swift:78` 使用 TaskSelectionClearingScrollView；`WorkspaceShellViews.swift:249–276` 是 GeometryReader、ScrollView 与 content，没有 ScrollViewReader/scrollPosition；`NoonmarkStore+SelectionProductivity.swift` 的选择流程更新 workspace selection、selectedTraceID 与 detail rail，未见 Day 自动 scroll 请求（摘录在 `10-swift-readonly-scroll-source.log`）。因此“新行暂在视口下”是本轮真实观察，不据此称原版差异或新增自动滚动需求。Swift 未实际运行，静态读不能证明运行行为完全对等。

## 默认门禁与复跑

两个测试均在默认 `go test ./...` 中，无 build tag、无 skip。root 测试每次调用内部 fixture 测试生成全新目录后才 Open；生成、缺失或打开失败一律 Fatal。不依赖本 BUILD 的残留 session，不提交 session。现有 `scripts/check` 默认覆盖 root 与 internal/taskapp 的 race 测试及 vet；无需修改冻结入口。

已只读确认 CI 链路：`.github/workflows/mygo.yml` → `ports/mygo/scripts/ci/run` → `scripts/check`，因此后续合并新增测试后自然执行。当前没有运行或写入 GitHub；这里只证明仓库入口可达，不声称 hosted CI 已通过。CI 仍只上传 bounded `ci-evidence/*.txt`，本地保留的 session/PNG 不进入该上传合同。

保留原图及磁盘证据的独立入口：

```sh
export NOONMARK_MYGO_BUILD_ROOT=/workspace/shared/noonmark-carryover-year-review-20261106
export NOONMARK_GO=/workspace/scratch/586e77bd59a6/noonmark-mygo-work/go/bin/go
export GOPATH=/tmp/noonmark-gopath
export GOCACHE=/workspace/shared/noonmark-go-build-cache-20261105
bash scripts/test-carryover-year
bash scripts/check
```

所有命令从独立 `ports/mygo` 执行，BUILD 必须事先存在且在源码外。每次独立脚本建立新 evidence 子目录；成功自动生成该轮 SHA256SUMS，失败保留已产生原始资料并返回失败。日志、原图、session 的最终 SHA 清单位于外置 BUILD，不把运行资料写入源码。

## 不覆盖的内容

这是 365 日期负载下的中段 Day carryover 与今日 34 行滚动验证，没有逐日走完全年 UI。没有子任务复制/lineage、真实非空 catalog、原版完整一年故事或全功能跨季度重复使用验收；没有 SQLite、加密烛龙 sidecar、OS App、系统 IME、非 Linux 实机或 Swift 像素对账。本批不能替代以上门禁，也不能冒充 R7 冻结时已存在的证据。未做 GitHub 写入、commit、push、发版或清理共享 cache。

## R2测试设施修订（候选阶段记录）

以上R1运行数字与原图仍是R1证据，不回写为R2结果。R2另有runtime_carryover_year_process_test.go，将一次嵌套go test拆为有界编译及直接测试进程；原年度业务断言逐字保留。新增直接child生命周期测试首轮race7.186s、加强取消收束断言后的race7.182s通过。最终R2主进程 race 195.516s（年度本体188.30s）通过，内部fixture独立race49.499s及root/taskapp vet退出0。直接fixture普通进程10.56s，startedPID=waitedPID=197；实际8张原图另存新BUILD的evidence/final-native，整合阶段实际查看重开未来日图，布局、三个任务与已选目标一致。旧方式回退探针只把CommandContext换回Command，cancel子测试在3秒收束门禁实际失败，30.083s后由helper自身timeout退出并Wait；原断言不变，日志保留，不留下运行进程。独立process快速race7.215s亦通过；后续独立年度race186.955s及正式组合aggregate已通过，详见独立报告与本文末节；各项仍各自对应其实际执行范围。

明确未动态覆盖：后代继承stdout/stderr时真正触发WaitDelay；构建阶段失败后不得进入fixture的两阶段路径。前者有有限WaitDelay配置，后者有直接错误返回和不启动分支，均已静态审查，但不把静态审查称为运行证据。构建后代全部终止未作保证；实际session只由直接fixture child拥有。

## 正式组合验收（2026-11-06 UTC）

本节是后续正式副本的实际结果，覆盖本批、保存状态文案、年度R2与普通任务链纯核心的组合；前文候选阶段记录仍保留其原来范围。全部467个源码文件在完整检查、构建及CLI期间哈希/模式保持不变。

- 完整 `scripts/check` 退出0：应用主包race470.810s、taskapp101.273s、taskview1.052s、titlesession1.411s、domain core9.821s；四模块race/vet、固定Unicode16及51项依赖检查、MyGo输入/弹层定向门禁通过。inputdraft、titlecoordinator、textcanon、codec canonical为缓存结果，不称为新跑。
- `scripts/cross-build` 六目标全部退出0：macOS/Windows/Linux的amd64/arm64，只是编译，不是这些系统实机验收。主机构建退出0。
- 实际隔离Linux CLI创建、重开、day/pool渲染、会话复用拒绝及隐式scope拒绝均通过；没有打开真实桌面窗口。
- 证据保存在外置BUILD的 `combined-validation.json` 与 `combined-*.log/.exit`。GitHub写入仍待确认，未推送、没有此组合的新hosted CI；462d6da的旧候选发布复现不能套用到本批。完整产品、Swift实跑、系统IME/读屏及真实三平台仍未验收。

## 提交与案例状态

实现提交 `f1291ebe1033c5ad37a7ea3b43a838ca6046238c`；GO-TEST-YEAR-PROCESS-001 已在上述本地验收范围内闭环。后续文档闭环没有改变已验证Go源码。GitHub写入仍暂停；不把本地提交当作已推送或新CI成功。
