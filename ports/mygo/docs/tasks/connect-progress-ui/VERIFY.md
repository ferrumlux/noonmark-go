# 验证与交付记录

日期：2026-11-05（UTC）。工作副本：progress-ui-work/ports/mygo；源码基于当时正式树73aaaa1，加冻结的7个纯progress文件及独立review test。正式进度core后续commit为ee6e484；本候选没有覆写该正式树。

## 确认的行为

- 真实 Service＋原生 UI：简单完成／困难待办得到25%，不是1/2；难度调中等得到33%，完成所有子项得到100%但父项仍pending。用户显式父完成后遵循源manual100%、weights1/1，撤销后恢复weighted；撤销单个child与删除都会重算。
- 回池／重新排期后新trace只剩一个未完成child，仍从历史lineage得到25%；重开保持完全相同graph和进度。Pool本身不新增进度。删除池里的最后planned child再排期，当前trace虽无child，历史weighted仍显示且全chain manual guard仍拒绝；父完成仍由显式command决定。
- 两窗引用同一cache与projection。writer已持久但publication尚未post时，两窗仍读原25%；按FIFO先到33%、后到100%。重复确认快照、completion回调与多帧重绘均不重建，O(1)双窗row查询的AllocsPerRun=0。
- before-reserve、after-reserve、before-commit、after-persist四阶段失败均保留旧graph、旧projection实例与25%，错误可见。只有重开并验证after-persist持久事实后，才显示100%；不完整commit仍不能重开。
- 有效合成历史（明确不持久化、不冒充未实现命令）验证floor80%高于raw1/4，以及manual20/100仍显示floor80%。读条点击、左右／上下键不改写资料；没有RoleSlider或可调整动作。未知ID与无效graph不显示假0%。
- 自然日advance后重建一次；历史unfinished仍可显示25%；同日重复观察不重建。独立同步headless入口仅初始化／成功command／成功日期转换更新缓存。
- 行内仅0<percent<100且pending/deferred/unfinished。源150×4、gap6、top5、10pt semibold；详情13pt bold、tnum、右对齐、gap8、6pt。圆角采用Capsule半径height/2，完成绿色、其他accent、track为chip。2×真实像素检查圆角和条内颜色，最小960×720长标题／展开child／下一行不重叠。

## 命令与结果

环境：Go 1.27.3 Linux/amd64；外置NOONMARK_MYGO_BUILD_ROOT=/workspace/shared/noonmark-progress-ui-build-20261105，GOPATH=/tmp/noonmark-gopath，GOCACHE=/workspace/shared/noonmark-go-build-cache-20261105。每次Go调用先source scripts/environment，保留冻结MyGo依赖、patch身份与只读module选择。

- 15个TestProgressUI顶层测试＋TestFailureGateRegistry：`go test -p=1 -race -buildvcs=false . -run '^TestProgressUI|^TestFailureGateRegistry$' -count=1`通过，23.720s。日志evidence/progress-complete-focused.log。
- 最终scripts/check：exit0通过，evidence/final-check.log。应用main race112.591s；四模块的gofmt/race/vet均通过，部分未变更internal包使用有效Go cache。51项依赖准备测试21.777s通过；MyGo native ui2.709s与popup host1.052s通过。
- 六目标compile：exit0通过，linux/amd64、linux/arm64、windows/amd64、windows/arm64、darwin/amd64、darwin/arm64；evidence/final-cross-build.log。仅证明compile，不代表真实桌面运行。
- Linux CLI：exit0通过，evidence/final-cli.log。隔离development创建、重开、20-screen枚举、重复创建拒绝、无身份启动拒绝均通过；day／pool实际ui.Render为2400×1536。

## 真实headless截图

均位于外置evidence/progress-ui/，由MyGo Tester调用实际app/组件路径生成，不是HTML／CSS、静态参考图贴图或手绘界面：

- weighted-list-detail-2x.png：2400×1536，真实25%与展开child，两处进度一致。
- weighted-minimum-2x.png：1920×1440，短标题、进度、子项胶囊安全布局。
- long-parent-expanded-minimum-2x.png：1920×1440，长父标题、多行、展开子项与下一行。
- synthetic-history-floor-weighted-true-2x.png、synthetic-history-floor-weighted-false-2x.png：2400×1536，有效source-shaped历史floor合成图；不表示该历史命令或manual setter已实现。
- weighted-detail-component-2x.png、completed-green-component-2x.png、manual-zero-component-2x.png：600×200，实际详情组件、25%／100%／0%轨道与色值。

已查看主要整页、最小窗口及组件图，对照官方43e6eef的day-todo.png和完整TaskDetail.swift进度源码。没有同机Swift像素oracle、系统字体／SF Symbols逐像素对等、OS实机或系统IME／读屏验收。

## 捕获的缺陷与回归

GO-UI-PROGRESS-001记录行内进度被native Button的叶子语义隐藏，以及移为平级后点击被遮挡。两个真实红证据为progress-second-focused.log、progress-third-focused.log；第一轮全check使用修正前编译产物也捕获前者，full-check-first.log保留。根因修正为平级RoleProgress＋整个subtree IgnoreInput(true)。最终专项与完整check均绿，现有父标题Label未改，原五个无障碍测试与全部子项IME／草稿／multiline门禁在最终check通过。

## 整合边界

只交付main.go、runtime_title.go、runtime_ui.go、runtime_day.go、runtime_progress.go、runtime_progress_test.go，以及本任务文档和独立故障案例。registry仅追加docs/tasks/connect-progress-ui/gates-to-append.tsv这一行，绝不用整个候选gates.tsv覆盖正式记录。冻结owned manifest会记录各文件SHA-256。

没有改core／Service／validator／持久化／依赖patch；没有production、iCloud、Keychain、真实AI网络、安装或原MyGo修改，没有push或正式commit。当前所有manual展示都只读，后续setter工作不纳入本批。

## 冻结终态

2026-11-05 22:00 UTC，6个Go文件自frozen-go-manifest.json冻结后无变化；所有上述最终门禁针对相同Go SHA。最终manifest.json另列10个owned文件与唯一registry增量，134个domain／Service／依赖输入hash均未变。正式四个既有owned实现仍等于任务开始输入，正式树尚无本批runtime_progress.go；独立审核、正式整合和commit由父任务负责。

## 独立审阅与正式整合

随后独立副本完整 scripts/check 通过（main race102.299s），并新增5项永久顶层测试，race20.862s通过：深层clone与相同图复用、同日sequence与历史最大难度、跨窗发布保留父／子marked输入、33行长列表滚动后的完整进度覆盖区点击、未来日只读与成功no-op不重建。独立六目标编译和Linux CLI通过。外置Go overlay分别恢复按钮叶子隐藏及取消IgnoreInput，两个原症状门禁都实际判红；冻结实现为绿。

正式整合只复制10份owned和新增独立测试，并追加registry行，没有覆盖新版README、scripts/check或core。随后正式完整 scripts/check 退出0，main race129.591s；新增Unicode16完整入口仍通过；51项依赖准备测试22.371s、MyGo native ui2.765s／root1.044s。六目标应用编译与Linux development CLI退出0，day／pool实际离屏绘制仍为2400×1536。上述是本地真实门禁，不是GitHub新head CI或桌面实机验收。

正式回归保留作者6份Go文件原SHA及独立测试SHA 5101620f8d44498dfe94ada6d41c148b233646568613a6ccb7b77b8380d89019。当前GitHub写入待确认，没有推送本批或创建发行；手动slider仍未接通。
