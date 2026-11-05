# 日标题原生接线设计

2026-11-06，新加坡。实施前设计记录，仅合成 development 资料；不改正式仓库／MyGo／Swift，不推送。

编辑配方取 TaskDetail、DetailPrimitives、AutosavingMarkdownEditor 与 MacUIDesignContract：pending 且日期不早于今天，14pt600，透明无边框，垂直3水平0，22至72，多行 Return 换行，不增加保存／取消。完整 Markdown 仍未完成。历史只读采用同字号。

一个应用 hub 共用原协调器与 FIFO 写入 lane。另一 FIFO dispatch worker 才调用 RunOnMain，不在 build inline 调用，不用 Window.Update。build 仅建立稳定 string/Handle 记录及收集变化，真实 coordinator 调用在帧间。注册只排一次，关闭前同步补齐仍待注册的控件。只有变化请求 redraw，禁止 idle 每帧 post。

Native Snapshot/Finish 读真实 editor；Replace 比对最后 fresh proof 后才写 binding，再 Snapshot 同步并核实字节。失败取消关闭并保留来源。永不保留 Element、不回退 draft。

导航、日期、选择、收栏、返回统计、完成先请求 lifecycle transition；失败留在原详情。安全差异：Swift 可先导航再保留 retiring editor，此候选等待 flush 成功才离开。异步命令只捕获不可变 ID/date/title，所有 Service 写入共用 lane，在执行时采自然日。普通 blur 单独 EndEditing，不偷换源空白边界。

实际 Close 与 Quit 每次重新经过 native guards；全部窗口先检查再退出，ReleaseClosed 在实际 close 同阶段。非标题命令未完成也不提前关 Service。退出允许后才 drain 和 close。

lane 内取得的 Service 图投影按 FIFO 发到 UI，先 fresh native 同步再更新 clean title。午夜不在 build 写盘或清掉编辑器：结算后历史标题写入仍由 Service 拒绝；旧 dirty editor 留存、错误可见，不迁移 trace。clean 历史 editor 可安全转只读。

验证注入确定性 Post/clock/lane，真实 ui.Tester 与 Handle，不依赖 native loop；六目标编译不是三平台 IME／Swift oracle。实际系统 IME、强制杀进程及完整 Markdown 不冒称已验证。

## 实现与复审补充

- `main.go` 在 development App 中建立共享 hub／两个 FIFO lane，并接实际 OnBeforeQuit／OnClose。只读渲染与旧静态 fixture 不挂此 controller；新测试明确挂载实际编辑器。
- `runtime_title.go` 为原生薄适配器及 host。所有普通写入也经过同一 lane，进入 worker 前锁定标题、日期、trace／chain；同窗同一 quick-add 草稿在途拒绝重复提交，成功仅清除仍相同的原草稿。普通命令失败取消 Close／Quit 意图，不靠退出后 drain 才处理失败。
- 序号化导航保留最新同窗意图。旧 completion 即使早于新导航 post 执行，也只退役已证明安全的旧 editor，不清除新目标。取消、覆盖和关闭均释放闭包。
- 日界检测只负责排队；停止状态在 worker 执行前检查，永久错误锁存。一次失败不会被自己的 redraw 再次触发。临时日界失败允许后续明确命令重新尝试；标题临时错误继续使用原 Gate 调度。保留旧草稿时只刷新同 trace 的状态事实，不冻结为“待完成”，也不把草稿转交给新 trace。
- 标题错误属于具体 editor。成功重试仅清除该保存所解决的旧错误，不能把完成前新发生的原生读取失败隐藏为“已保存”；无关命令成功也不消除未解决标题错误。
- 纯 coordinator 的重复 Close／in-flight 反例由正式协调器批次修复后同步：已有 flight 时保留候选，等待旧 completion，不把 BeginPersistence 的 ErrNotReady 当作保存失败而清除 Close 意图。

## 测试范围与证据

新增 49 项顶层 UI／独立复审测试（另含导航、原生输入、磁盘阶段子案例），采用真实 `ui.Tester`、实际 Handle 与合成 Service。覆盖700ms重开、多行 Return、IME中间态、在途更新、普通空白blur与lifecycle区别、任务／日期／页面／收栏／picker转移、完成次序、重复Close、初帧注册缝隙、帧前原生commit／marked选区替换、Replace新输入拒绝、陈旧generation、跨窗quit与已确认文字同步、历史失败留稿与clean退役、30次idle frame不排新工作、quick-add重复／失败／后续草稿、停止后不写／不自触发循环、导航闭包及时释放。

四个实际磁盘fault阶段均经编辑器保存／Close入口验证：before-reserve保留旧磁盘；after-reserve／before-commit重开fail-closed；after-persist旧handle不确认、重开核实新字节。测试专用反射／unsafe只用于访问已有headless native client与合成store fault seam，没有新增生产绕过接口或修改依赖。

外置证据根：`NOONMARK_MYGO_BUILD_ROOT/evidence`。`title-ui-focused-final.log` 为完整新增race测试；`title-native-ui.png` 为实际运行编辑器截图（2400×1536像素、2×倍率，14pt600、透明无边框、72DIP内滚动）；`title-native-ui-saved.png` 为同一Tester短标题保存成功后截图。独立review用例保留原断言后纳入测试，仅gofmt及必要fixture适配：合成资料建立后明确发布到UI读视图，不让fixture绕过实际缓存。初版检查与六目标曾通过；首次CLI重跑因漏传脚本要求的binary变量拒绝执行，随后通过scripts/build产出并显式传入，不绕过脚本。

最终聚合、六目标及CLI结果以 `title-ui-check-final.log`、`title-ui-cross-final.log`、`title-ui-build-final.log`、`title-ui-entry-final.log` 为准；这些验证在最终源码上完成后才作为交付通过证据。未重跑已知基线allocation失败的六分钟框架全量race；支持的聚合仍含37项原生定向回归与50项依赖prepare门禁。

未验证：真实macOS／Windows／Linux系统IME与系统退出、Swift动态oracle、非Linux实机、完整Markdown、生产资料／iCloud／Keychain／AI。六目标编译与Linux headless成功不代表这些项目完成。

## 重绘不等待磁盘

共享hub在首次写入之前取得已确认graph／today，后续仅由worker的FIFO publication在UI线程更新。`titleReadView` 的Snapshot深复制缓存，Today只读缓存；写方法只是转发真实store且只在writer执行。新窗口也通过此读视图初始化，避免另一窗口事务持有Service.mu跨磁盘提交时阻塞窗口创建或重绘。Service依然拥有全部写入合法性检查。

新增真实Service fault barrier回归：事务持锁停在before-reserve，另一串行UI执行序列可读取Snapshot／Today、重绘并初始化第二窗口，看到的仍是旧已确认图；释放worker后、交付publication之前仍不提前显示新图，之后才更新。超时只用作死锁失败保护，不作为原生输入屏障或成功时序证据。

## 普通异步动作的重复与迟到完成

任务池弹窗每次展示分配新epoch；排期完成捕获epoch、日期、页面与选择，只有仍匹配的当前弹窗才关闭并选中结果。取消后重开不取消已接受存储事务，但旧完成不会操作新的弹窗或选择。

共享hub按操作种类＋chain／trace＋日期／目标完成状态抑制同一在途动作的重复激活，包含跨窗口；不同合法动作继续FIFO排队。成功或失败都释放该键，不把去重当作永久禁用。实际Tester回归覆盖取消→换日期→重开、弹窗连点、任务池今天／明天按钮连点、已完成列表撤销连点，以及跨窗口同动作去重但独立命令仍执行。

## 启动失败的退出状态

事件循环返回错误时先排空writer／dispatcher并关闭Service，再以非零状态退出；成功仍为0。新增 `runtimeExitStatus` 单元回归核对清理先于失败状态返回。此分支没有通过真实桌面事件循环触发，不能将helper测试说成系统窗口启动／退出验收。

## 最终结果

最终源码 `scripts/check` 全过：应用主包race 46.888s；其余领域／应用包race、全模块vet／gofmt、50项prepare门禁及37项prepared-native定向回归通过（原生回归race 1.893s）。49项新增UI／复审测试加1项退出helper focused race通过12.858s。macOS／Windows／Linux amd64／arm64六目标编译、支持入口build与CLI development新建／重开／拒绝复用均通过。两张2×实际Tester截图已目视核验。以上均为隔离合成资料；真实系统IME／桌面退出及Swift oracle仍未运行。


正式副本复验（2026-11-06，新加坡）：显式21文件SHA核验后集成，保留已备份协调器修正；补充GO-RUNTIME-007／008及门禁映射。初次聚合如实被TestFailureGateRegistry阻断，因为原白名单尚未包括新titlecoordinator包；扩展到实际受scripts/check测试的新输入包后重跑，未放宽任何缺失测试断言。最终应用主包race45.689s、各模块race／vet／gofmt、50项prepare、37项native race1.856s、六目标编译与CLI全部通过。正式外置证据：evidence/title-native-ui-{check,cross,entrypoints}-final.log。两张正式Tester截图与隔离候选逐字节一致。
