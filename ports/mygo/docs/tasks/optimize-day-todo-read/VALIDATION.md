# 候选验证记录

## 身份与实际命令

来源、精确两文件old/new见同目录manifest/overlay。全部运行在独立候选；旧ProjectDay对照在另一个未改产品的3346本地clone中，只复制同一新benchmark测试文件。正式与I1未动。

环境：Go 1.27.3、Linux amd64、AMD EPYC 9V74；GOMAXPROCS=2，GOGC=20，GOMEMLIMIT=700MiB。每个命令先设置外置 `NOONMARK_MYGO_BUILD_ROOT=day-read-build`，source `build-environment/activate.sh`，设置 `GOCACHE=subtask-integration-build/go-cache`。下列路径相对外置BUILD；精确最终命令亦保存为 `.command`、exit为`.exit`，原输出为`.log`。

在 `ports/mygo/domain-core`：
- `go test -race ./core -run 'Test(SnapshotIsDeeplyDetached|ConcurrentCreationAndReads|DayTodo|TitleCancelledDraft)' -count=1`：exit0，1.020s（focused-initial.log）。
- `go test -race ./core -run '^TestDayRead(Nil|Public|History|Sorting|Command|Allocation)' -count=1 -v`：semantics-focused-race-final.log，exit0，1.154s；ownership独立 `-run '^TestDayReadOwnership'` exit0，2.786s（ownership-focused-v3.log）。最终完整套件覆盖全部新增测试。
- `go test -race ./... -count=1`：exit0，core13.770s、textcanon1.640s（core-full-race.log）。
- `go vet ./...`：exit0（core-vet.log）。
- `go test ./core -run '^$' -bench '^BenchmarkDayReadAB$' -benchmem -benchtime=10x -count=3`：exit0，0.295s（semantics-bench-final.log）。
- `go test ./core -run '^TestDayReadOwnershipWriterObservation$' -count=1 -v`：exit0，0.490s（writer-observation-v3.log）。

在 `ports/mygo`：
- `go test -race ./internal/taskview -count=1`：exit0，1.053s（taskview-final-race.log），保留排序、stats/review、recurring read-only、非法图及日历排除既有测试。
- `go vet ./internal/taskview ./internal/taskapp`：exit0（related-vet.log）。
- `go test -race ./internal/taskapp -run 'Test(HistoryEditFrozenProduction|DayReadFrozen|DaySnapshotFrozen|DayDetailFrozen|OutcomeFrozen)' -count=1`：exit0，1.042s（freeze-final-race.log）。
- 新旧各 `go test ./internal/taskview -run '^$' -bench '^BenchmarkDayReadProjectDay$' -benchmem -benchtime=10x -count=3`：exit0（projectday-baseline-bench-bench.log、projectday-candidate-bench.log）。
- 新增/修改Go文件 gofmt检查与git diff --check无输出、exit0。

调试失败不隐藏：ownership初始fixture出现类型错误、同日priority重复、错误命令日期/clock；修正测试fixture后真实重跑通过。可保留失败输出在ownership-focused-fixture-failure.log与ownership-focused-debug-failed-v2.log；早期若被迭代覆写的stdout不声称完整留存。产品两文件未因这些失败改变。语义benchmark初版无关notes/identity随返回参数变化，最终版固定无关图并加DeepEqual断言后重跑，最终报告只采用final日志。

## 分配与时间样本

独立旧oracle与新DayTodo，选中4行×2notes不变，无关图包含其他日期traces、chains、definitions、subtasks与notes：

- small无关0：旧5848–5849B/45alloc，新2440B/22alloc。
- medium无关100：旧103512–103524B/845alloc，新2440B/22alloc。
- annual形状无关3650：旧3381900–3382450B/29245alloc，新2440B/22alloc。旧4.936–5.582ms，新44.383–66.325µs。
- 无关100固定，返回40行：新41296B/177alloc；返回4行、每行20notes：新6888B/58alloc。测试独立确认无关实体未改变。

新读取消除无关深层复制分配，CPU仍两次O(T)扫描，不能称O(1)。年度形状微基准不是实际年度开发路径。

ProjectDay源码不变，相同命令合成fixture：
- 1日：旧18448B/127alloc，新11056B/71alloc。
- 30日：旧约392.7KB/569–570alloc，新168.6KB/512–513alloc。
- 365日：旧4369040–4369146B/4658–4659alloc，新1616081–1616239B/4596–4598alloc；旧4.167–6.943ms，新2.174–2.383ms。

这仍保留整图验证/私有clone/构造验证，仅缩小内部重复DayTodo读取。样本并非root热点占比或CI保证。

## writer观察与局限

2reader、100无关行、60次合法manual-progress写命令，三次old/new平均：1.293/0.964、1.590/1.324、1.313/1.164ms。测的是命令整体耗时，包含clone/validation/调度，不是纯mutex等待。第三组new最大3.797ms高于old2.156ms，不承诺尾延迟无回归；无时间阈值、无GC调整。

初始微基准阶段尚未运行原完整聚合/年度/六目标；下节记录随后获准的独立串行实际运行。hosted仍未执行。没有删原测试或放宽timeout；本地单次绿不证明性能风险消除。当前不push或正式合入。


## 独审后原入口串行验证（2026-11-06 UTC）

实际被测commit为 `f541dc28f4b6d0e13da36f38151424fe342f8e44`，tree `b4fab672367710f65a86c231a11fdd4a66a150c8`，所有阶段起点与结束均clean。先固定外置 `day-read-full-validation/VERIFIED-f541dc28.json`，对所有已跟踪文件逐一核SHA，整个运行前后字节相同；本文是随后独立文档checkpoint，不把后续文档commit冒称已被重型运行。

使用新外置 `NOONMARK_MYGO_BUILD_ROOT=B/day-read-full-validation`，source原activate，GOCACHE复用已授权 `B/subtask-integration-build/go-cache`、GOPATH原固定依赖，未删除cache。固定GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB，未加CPU/mem profile。保存环境、源清单、原脚本字节、命令、exit与日志hash。可见nproc=9、内存约9.7Gi；lscpu受环境/sys不可用限制，未虚构CPU拓扑。初次外置计时器因/usr/bin/time不存在exit127，原年度脚本未执行，错误保留annual-launch-error.*；改为Python标准库计时后才首次执行实际年度。候选源/原脚本均未因计时调整。

按原入口逐项串行，四项真实exit均0：

1. `bash scripts/test-carryover-year`。原脚本执行 `go test -p=1 -race -count=1 -buildvcs=false -timeout=15m -run='^TestCarryoverYearMiddleNativeService$' -v .`；15m是该年度专用入口既有参数，未改聚合默认600s。test179.49s、root包180.528s、脚本wall221.446s。真实fixture进程生成365日/762任务，native长滚动、defer/continue、关闭重开完成，762→764任务、revision2→4、native_close2、reopen_equal=true。headless native Tester、真实SerialLane/Service、隔离development，不是OS实机。原始证据 `day-read-full-validation/evidence/carryover-year-TCBxwV`，内含SHA256SUMS。
2. `bash scripts/check`。未加筛选、timeout、count或cache绕过参数，沿用原四模块gofmt、race test、vet、canonical16、dependency与native入口。整体wall1041.086s，root实际非cached557.039s，taskapp259.566s，core14.994s，mygo/ui73.210s。全部完成exit0，默认root timeout仍600s，本次名义裕度42.961s。不能与不同环境的正式hosted592.886s作受控A/B，也不能保证下一次裕度；单次本地通过不是hosted证明。
3. `bash scripts/cross-build`。wall137.684s，原六目标linux/amd64、linux/arm64、windows/amd64、windows/arm64、darwin/amd64、darwin/arm64全部编译成功。只编译，非六平台实机；产物hash外置binary-sha256.json。
4. `NOONMARK_MYGO_BINARY=$BUILD/bin/noonmark-mygo-linux-amd64 bash scripts/test-entrypoints`。wall1.209s，原Linux隔离development session新建、Day与Pool重开渲染均2400×1536，reuse与implicit入口拒绝护栏通过。没有desktop window实机验证。

各阶段完整日志为外置 `annual.log`、`check.log`、`cross.log`、`entrypoints.log`，配套`.command/.exit/.started/.finished/.time.json`。来源和log hashes绑定在VERIFIED-f541dc28.json，原脚本副本在original-scripts/。正式3346与Snapshot I1未改，没有push、正式集成或新hosted run。

独审额外race writer三组观察：new最大7.2–9.6ms，old5.1–6.3ms，均值略降。这是小样本整体命令耗时，不能当纯锁等待或统计定论，不宣称writer尾延迟解决。后续安全同步、新head CI与正式合入决定仍属于下一阶段。
