## 当前状态（2026-11-06，新加坡）

日待办 pending 且 date>=today 的原生多行标题编辑已接入隔离 development 应用：700ms自动保存、真实Handle快照／最终化、统一写入lane、可取消Close／Quit及失败留稿。完整实现、最终证据与安全差异见 [NATIVE-UI.md](NATIVE-UI.md)。下面报告保留为当时的前置设计／验证记录，其中“只读／尚未开放”等描述只适用于该历史批次，不代表当前运行状态。

Linux headless与六目标编译不是实机IME或Swift动态oracle；这些限制及原始未修复依赖红色探针继续保留。

本报告为接线设计与原始反例，尚未启用 UI。后续隔离 overlay 的最小 focus-finalization 候选已令三个原始探针转绿，但未集成锁定依赖，也不证明完整退出流程；以 VERIFY.md 与 GO-RUNTIME-007 的当前状态为准。

# MyGo 任务标题 autosave 接入设计与已复现阻断

日期：2026-11-05。状态：设计完成，UI 不可开放，原生输入最终化仍未修复。

本报告只读研究正式 `noonmark-go/ports/mygo`、上游 `noonmark-reference`（43e6eef）及 MyGo（977ea57e0baf）。只在本目录写报告、独立探针与 Go overlay；没有修改正式仓库或 MyGo 原仓库。运行的是 Linux 内存 renderer／原生 client 模拟，不是 Swift oracle、真实输入法或跨平台实机验收。没有 production、iCloud、Keychain 或 AI 调用。

以下路径前缀：`N/` = noonmark-go/ports/mygo，`S/` = noonmark-reference，`M/` = mygo。行号来自本次只读源码；同批实现已在 N/internal/taskapp/title.go 加入 trace 路由，因此用最新内容引用该文件。

## 先说结论

1. `TextAreaBase`、`Element.Composing()`、`FocusLeft()`、`Context.Now()/After()`、`Window.Update()`、`OnClose`、`OnBeforeQuit` 足够承载普通输入、700ms debounce、后台事务和可取消退出。
2. 它们**不足以直接证明最后一笔原生输入已经进入应用绑定**。原生 `TextInputClient.ReplaceText/SetMarkedText` 可先修改 editor，直到下一次构建才更新 `*draft`；这条原生 client 路径不经过 `ObserveInput`。立即关闭时直接保存应用 draft 会漏字。
3. “PreventDefault → 保留控件 → Root.Focus → 再建一次 → 保存”的简单两阶段方案已被实际反例击穿。用 marked text 替换旧标题选区、还未建帧就关闭，结果 captured=""，应为"未定"。一帧内部可有多个 build pass，第二 pass 的 `applyString` 清掉 composition，早于帧末的 `release/UnmarkText`。不能把最初两个通过的简单探针当作安全屏障。
4. 正式标题 Service 现在已有 `SaveTraceTitleInput(traceID,title)`：selected trace 检查与完整标题事务持有同一锁，避免旧历史 trace 回调改同链的新 pending。UI 不再接链级 `SaveTaskTitleInput`。自然日刷新仍要在写入前完成，不能用 UI 快照检查替代 Service 锁内检查。
5. 建议先补最小 MyGo 原生文本快照／最终化能力及焦点切换缺陷门禁，再接 UI；不添加原版没有的 Save/Cancel 按钮，不改为单行 Return 提交，不借此实现大 Markdown parser。

## 1. 原版事实与视觉配方

- S/App/NoonmarkMacApp/TaskDetail.swift:24–47：只对 `pending && trace.date >= today` 开放标题；owner 为 `task:<chainID>:title`，回调捕获 traceID。
- S/App/NoonmarkMacApp/DetailPrimitives.swift:164–183：`.title`、`showsSurface:false`、`.nonemptyTrimmed`；只读为 MarkdownText，空标题显示“无标题”。
- S/App/NoonmarkMacApp/MarkdownEditing.swift:16–31、45–74、99–117：标题 14pt semibold、允许多行，文本 inset 不是卡片 padding。
- S/Sources/NoonmarkMacUIContract/MacUIDesignContract.swift:721–727：标题高度 22…72，水平 inset 0、垂直 inset 3、line-fragment padding 0。
- S/App/NoonmarkMacApp/AutosavingMarkdownEditor.swift:31、157–195、221–299、302–405：Return 不提交；结束编辑／消失 flush；非空才保存；空白 debounce 保留本地，结束编辑恢复 lastAcknowledgedText；失败保留草稿；成功回调不能盖掉更晚输入；退出等待在途存储，没有超时后假成功。
- S/App/NoonmarkMacApp/NoonmarkMacApp.swift:368–432：退出先 unmark 当前 NSTextView、结束编辑，异步 flush，失败取消，成功重入 terminate。
- S/Sources/NoonmarkMacRuntime/InputDraftFlushCoordinator.swift:23–58：owner+registration token 注册，只有匹配 token 可注销；flush 对注册快照逐一等待。

建议原生控件配方（不是已落地 UI）：

```go
edit := ui.TextAreaBase(c.Key(titleElementKey{sessionID, ownerID, generation}), &session.Draft).
    Width(width).FontSize(14).FontWeight(600).TextColor(ink).
    Padding(3, 0).Lines(1, 1000).MinHeight(22).MaxHeight(72).
    Background(ui.Transparent).Border(0, ui.Transparent).
    Label("日任务详情标题").Placeholder("任务标题")
```

`Lines(1,1000)` 只让自然高度按实际换行增长，最终以 72 DIP 封顶并使用 textarea 自身滚动；不是允许输入1000行的业务限制。也可按实际行高导出足够大的 maxLines，但不能用固定3行高度代替 22…72。M/ui/editor_layout.go:78–94、layout.go:483–505 证明先求 wrapped 高度再做 Min/Max clamp。字号、fallback 与显示/编辑状态须统一。N/runtime_day.go:300–311 现为13pt只读，接入时删掉过时“标题尚未接通”文案，但保留子任务、分类未接通事实。不得加背景卡片或 Save/Cancel。

只读 Markdown 的完整对等仍属独立缺口；可复用已评审的原型/inline renderer，不得为了标题 autosave 偷渡新大型 parser。截图只验证配方，不证明持久化和 IME 正确。

## 2. MyGo 实际事件时序

### 普通 build 可用的接口

- M/ui/value_input.go:5–14、39–52：public constructor 返回时应用输入。绑定值随后可读。
- M/ui/textinput.go:26–62、281–288：TextAreaBase 背后为内建 multiline editor；Composing 读非空 `ed.compose`。不能只在 Changed 时通知 gate：composition 开始或结束可能不改 committed draft。
- M/ui/value_input.go:243–279：nativeDirty 在构建中合入绑定，Changed 随后产生；非当前焦点会清 composition。
- M/ui/focus_events.go:3–43、55–70：FocusLeft 依赖已构建控件，并可能在后一个 pass 才报告；整个控件被移除时不能靠它补救。
- M/ui/frame.go:7–9、63–65、86–106：Element 只在当前 build pass 有效。禁止把它塞进 goroutine、timer、service completion 或 close callback。持久身份用 Handle；目前 Handle 没有公开文本快照或 composition 最终化查询。
- M/ui/context.go:487–496、522–524：Now 是当前帧时钟，After 只请求未来帧。它不是回调定时器，也没有给每个 schedule 返回 cancel handle。

每个可见标题 build 的顺序应是：构建 textarea → 读绑定及 Composing → 由文本比较计算 textChanged → 调用 gate.NativeSnapshotDidChange → 检查 FocusLeft → 处理 flush 请求 → 检查到期 schedule。不得在构建前根据旧 binding 先执行到期保存。

### 原生回调不是公共 Tester.Type 的完整等价物

M/ui/editor_input.go:11–14、32–55、176–208、211–265：preedit 是虚拟插入；ReplaceText 修改 editor，publish 标记 nativeDirty，不同步写 string binding。SetMarkedText 替换选区时可删除 committed 选区，但新 marked 字仍不进入绑定。

M/ui/input.go:1093–1116 的普通 surface event 路径会先 ObserveInput；但真实平台可以直接调用 client：

- macOS：M/internal/darwin/surface_input.go:128–131、157–160；
- Linux：M/internal/linux/surface.go:1265–1269，preedit-end:1273–1279；
- Windows：M/internal/windows/surface_input.go 的 client.SetMarkedText 路径。

这些 direct client 调用经 M/ui/text_input_client.go:127–143，不进入 ObserveInput。故 ObserveInput 的 input epoch 不能作为跨平台“最后输入已捕获”证据。

公开 Tester.Type/Compose 每次都会 settle（M/ui/headless.go:366–373、838–862），它们适合正常输入测试，却会掩盖“原生输入与关闭之间无 frame”的缝隙。该缝隙必须用框架 overlay/private client 或真实平台症状测试覆盖。

## 3. 本次可重跑证据

### 公共 MyGo Tester 探针：通过

文件：`probe/probe_test.go`；输出：`probe-output.txt`。

验证了：

- composition开始而文本不变；选区替换时 draft 变空但 composing=true；清 preedit 时 composing=false但文本可不变；最终 Type 后才有候选提交。
- Enter 插入换行，不当成保存键。
- Context.After 配合 Tester.Advance，在699ms不保存、重打字重计700ms、到期恰好一次。
- pointer/tab/window blur 会通过现有 text-client release 提交 marked text并报告 FocusLeft。
- 直接移除 editor：binding仍为旧值，FocusLeft没有机会执行，证明生命周期不能依赖已消失控件。

### 原生 client 时序：通过的诊断探针

文件：`framework_probe_test.go` 的 `TestNoonmarkNativeBeforeFrameProbe`；使用 `framework-overlay.json` 注入测试，未修改 M 源码。

实际输出：

```
native ReplaceText BEFORE frame bound="old" observerCalls=0
AFTER frame bound="old已提交"
native SetMarkedText BEFORE frame bound="old已提交" observerCalls=0
```

### 简单两阶段排空：存在已复现失败，保持红色

同文件 `TestNoonmarkStagedCloseDrainProbe`；输出 `framework-probe-output.txt`：

```
committed-before-frame: captured="old已提交" want="old已提交" PASS
marked-before-frame: captured="old未定" want="old未定" PASS
marked-selection-before-frame: captured="" want="未定" FAIL
staged drain lost input
```

原因链：

1. 原生 SetMarkedText 指定旧标题选区，editor 删除选区并设置 preedit；binding 尚旧。
2. 第一 pass 构建把删除选区送入 binding，Changed 使引擎需要额外 pass。
3. 方案调用 Root.Focus；第二 pass 再构建 textarea，M/ui/value_input.go:278–279 因 focus不同直接 `ed.compose=""`。
4. M/ui/runtime.go:667–721 允许同一帧多pass，真正的 updateTextInput 在794；M/ui/input.go:1233–1237 此时才释放旧 client。
5. M/ui/text_input_client.go:98–102 已找不到 marked range，无从 UnmarkText；最新文本永久丢失。

所以失败不是“再等一帧”可修复，也不是时间太短；composition已被删除。测试应保留为未修复的 symptom gate，不把期望改为空以变绿。两个简单成功 case、Disabled 拦截迟到 client 的成功也不能覆盖这个反例。

正式原生 client 红色探针已移至 `probes/native_input_test.go.txt`，用 `scripts/probe-title-native-input` 重跑；脚本保留失败退出码。其他独立探针证据留在外置验证目录，不作为当前生产 API。

## 4. 最小框架修复／扩展建议（均未实施）

### 4.1 修正焦点切换顺序

不能先清 compose，后期望 release把它提交。内建 editor 的焦点变更应在任何下一次 applyString 清理前完成一次定义明确的 unmark/finalize，并把提交后的文本发布给绑定。处理必须覆盖 Element.Focus、Handle.Focus、Tab、pointer、window blur、控件移除与关闭；不能只在 Noonmark 的调用点补丁。

最小源码修复候选是集中焦点转移处完成旧 client finalization，再切 focus／撤销 input route；或把 applyString 的丢弃分支改为正确的 commit+publish+绑定交付。但不能只把第279行替换成 commitCompose 就宣称完成：commit发生后必须在同次调用把 nativeDirty合入绑定，并保留取消composition的明确语义、undo transaction、readOnly/disabled拒绝语义。移除时也必须把最后一次文本交给所有者，不能只改已经将被丢弃的 editor 内存。上述失败探针及 existing IME undo/cancel测试一起决定具体方案。

### 4.2 增加稳定、可在帧外调用的文本最终化边界

建议在 `ui.Handle` 上提供最小接口（以下为提议，不是当前可调用API）：

```go
type TextInputSnapshot struct {
    Text string                // 已完成的原生文本；最终化后包含marked提交结果
    Composing bool
    Revision uint64            // 实际editor文本/合成状态单调版本
}
func (h Handle) TextInputSnapshot(c *Context) (TextInputSnapshot, bool)
func (h Handle) FinishTextEditing(c *Context) (TextInputSnapshot, bool)
```

契约：主线程可在build外调用；关闭/未绑定/已替换身份返回false；读取真实editor而非上一帧string。FinishTextEditing原子地排尽该editor已有队列、unmark、发布绑定、结束当前文本路由，返回最新快照；未能安全结束则false，不能丢弃草稿后声称成功。应可在occluded窗口工作，不依赖下一次绘制。

框架内部应复用 widgetTextInput.prepare/UnmarkText/publish 的现有行为，增加可验证的binding交付；不要在应用里复制 editor parser或猜测marked string。native route必须失效，旧client在最终化、dispose、重新绑定之后不可以编辑新session；必要时用adapter identity/epoch，而不只复用相同元素ID。

平台层需让系统IME结束旧会话并校验晚到commit不重复插入。M/ui/text_input_client.go:87–95已有焦点/adapter身份验证，可扩充；macOS NSTextInputClient、GTK preedit-end/commit顺序与Windows路径要分别测，不把headless行为冒充原生IME对账。

本接口同时解决：帧前最后字符、尚未绑定的composition状态、没有可见frame的窗口flush，以及旧native client失效。它不是新增用户操作，也不要求弹保存确认框。

### 4.3 现有API不能替代的原因

- Window.Update是异步主线程dispatch，不是同步build完成；Window.Invalidate只请求frame。
- Handle.Focus也是请求焦点，M/ui/ref.go:181–201在build中应用；同样可能进入上述多pass缺陷。
- Window.Blur只让整个OS窗口失焦，不是指定editor成功最终化的返回契约；不应为了保存标题偷改整个应用焦点。
- CapturePage会走渲染/截图路径，不是文本事务API；不能把强制截图当保存屏障。
- M/ui/runtime.go:962–981会暂停被遮挡窗口的非input frame，进一步说明“等下一帧”不能保证退出完成。
- 未发现已公开的同步文本flush、原生composition快照或指定editor的EndEditing接口；这不是说MyGo没有异步dispatch。

## 5. 与独立纯 Gate 的适配，不重复实现状态机

候选纯包为 input-draft-work/internal/inputdraft，独立实施。使用它的实际API：

- NewWithGeneration(ownerID,generation)
- NativeSnapshotDidChange(textChanged,isComposing)
- RequestFlush / PermitsAutosave / BeginPersistence
- PersistenceDidSucceed / PersistenceDidFail
- DiscardLocalChanges / AcknowledgeWithoutPersistence
- IsComposing / HasUnflushedChanges / HasPersistenceInFlight

Schedule含OwnerID/Generation/Revision/Sequence/DelayMilliseconds；PersistenceRequest含OwnerID/Generation/Revision/RequestID。不能因为schedule被返回就认为获准保存，deadline时仍须BeginPersistence校验。

应用adapter拥有文本而非Gate拥有：

```go
type titleSession struct {
    SessionID uint64
    OwnerID string // task:<chainID>:title，与源一致
    TraceID core.DayTraceID // 回调不可事后改读runtimeSelection
    Gate *inputdraft.Gate
    Draft, LastNativeText, LastAcknowledgedText string
    PendingPersistedText *string
    CanonicalizeAfterSuccess bool
    Scheduled *inputdraft.Schedule
    Due time.Time
    Element ui.Handle
    Persisting bool
    Retiring bool
}
```

外层维护单调session身份和owner generation。Gate初始generation默认1不能跨实例重用来消除旧callback：旧实例相同owner+generation可以产生相同token。每次注册/替换须分配生命周期唯一sessionID/generation；所有completion先定位具体session，再交回原request。旧owner注销必须匹配registration token，不能用相同owner误注销新实例。

调度用c.Now+After；`Scheduled`被替换或清空即逻辑取消，遗留早醒frame只重新校验。0ms flush直接在当前安全UI阶段尝试，不依赖After(0)下一毫秒。必须在根controller推进所有仍注册session，不能只在当前可见textarea执行timer，否则导航后retiring草稿永远不再保存。

后台工作只捕获不可变 `{sessionID, request, traceID, normalizedText, rawDraft}`。用单一有序写入lane或每session最多一个in-flight，实际service已锁但不能让多个goroutine竞争改变同一session的输入顺序。Worker不碰Draft/Gate/selection/反馈。完成通过Window.Update，或goroutine调用RunOnMain再Invalidate；均回主线程处理。

M/content.go:166–191：Window.Update异步postMain，不等待fn。M/loop.go:82–109：RunOnMain同步等待，主线程直接执行，loop停后丢弃。M/ui/services.go:9–12明确Services.Invalidate不是模型更新锁。任何main-thread callback不得等待worker而worker又等待RunOnMain。

成功仅确认本request对应的normalized输入；如果Draft已更新，不写回旧raw/normalized。仅结束编辑且当前Draft==capturedRaw才canonicalize。service存在canonical等价no-op并保留原字节，因此确认值最好取成功后同事务结果或已提交快照里的实际标题，而不是假设输入字节必已落盘。

空白debounce不调用service、不把dirty假装保存；blur/lifecycle确认最终化后恢复LastAcknowledgedText并DiscardLocalChanges。已有in-flight要先完成再决定恢复哪个已确认值，不能恢复过时值。内部换行保留，CRLF/CR规范化按现有领域实现；Foundation whitespace精确对等仍未有Apple oracle。

## 6. Service／自然日／历史路由

最新版N/internal/taskapp/title.go:47–73已经实现SaveTraceTitleInput：在s.mu内找传入trace，先拒绝过去日期，再拒绝非pending，再调用editTitleLocked，由该trace解析chain。editTitleLocked:75起沿用完整候选、专用title历史审计、normalize、disk commit与成功发布。UI错误无权改写runtime snapshot。

这是真正的事务侧路由保护，不是“UI看见还是pending”而后另开锁保存。集成时须将其race/历史continuation负例纳入正式门禁；本报告不冒领其他测试结果。

写入前保留N/runtime_ui.go:106–113的refreshDay语义，但不能把现有runCommand整个搬进worker：它修改UI状态。拆成worker执行 `AdvanceTo(sampledToday)` 成功后 `SaveTraceTitleInput(capturedTraceID,text)`；UI完成回调再发布日期导航/错误消息。另一AdvanceTo若夹在两调用之间，SaveTraceTitleInput锁内检查仍拒绝历史trace，绝不退回链级保存。若要避免后台排队跨午夜的旧时间采样，应在worker执行写入时采样当前自然日，而非700ms timer创建时采样。

不把“结算+标题”伪称一笔原子提交：目前AdvanceTo与title是两笔；结算成功后title因原trace变历史而失败是合法状态，草稿必须保留且错误清楚。若产品要求两个动作单笔，则需另外设计组合事务与审计，不在本标题adapter暗中扩展。

runtimeView现先refreshDay再构建详情（runtime_ui.go:200–208）；observeRuntimeDay还会清runtimeSelection（423–440）。集成必须把自动移除改为controller请求的状态转移，先保留正在输入的旧session及native最终化，再更新可见投影。午夜以后不能“为了救草稿”回填昨日历史；保留未保存输入供用户处置，明确拒绝原因，不静默迁往同链新trace。

## 7. 所有离开路径统一走转移协调器

现有会直接清/换详情的入口包括：N/runtime_day.go:22–32日期，258–260任务选择，290收rail，308返回统计；N/runtime_ui.go:172页面切换，423–440自然日；完成任务runCommand也会让编辑能力消失。它们不能各自先改selection/page，再期望onDisappear补存。

建议 `RequestTransition(target)` 捕获具体目标和当前session；原native editor在最终化完成前仍挂载。最终化成功后普通导航可以保留retiring session异步flush并呈现目标，registry直到成功才注销；若无法在隐藏后清楚保留/恢复错误草稿，第一阶段应让失败取消这次转移，保持原详情，不能为了“导航流畅”扔掉输入。

后一种是安全实现策略，不宣称Swift有阻止导航的相同行为。Swift实际onDisappear发起异步flush且失败保留registration，见AutosavingMarkdownEditor.swift:181–194。若选严格源码行为，就实现app级retiring registry、重新打开旧owner恢复草稿、全局错误指向原trace，并测试失败时可取回输入。

再编辑同一owner时不得在旧session未结束时直接创建相同generation；复用既有session或明确串行retire后新建。目标变更反复发生时，只执行最新明确转移意图，但不能取消已开始的durable write或抹去其结果。

## 8. Close / App Quit 的可执行状态机

M/window.go:863–878与1841–1846：OnClose可同步PreventDefault，Destroy绕开事件；M/app.go:242–271、519–531：顺序为OnBeforeQuit→各Window.OnClose→OnWillQuit→停止loop→OnQuit。OnQuit太晚，OnWillQuit也已关闭窗口，不能用作首次草稿flush。

在完成4.2最终化接口后：

1. OnClose或OnBeforeQuit主线程立即PreventDefault，记录closing/quit intent；不等待磁盘、不sleep、不Destroy。
2. 对全部相关session调用原生FinishTextEditing，送真实snapshot给Gate。调用失败或仍composing则取消关闭并保留输入，不发domain写。
3. 取消debounce、RequestFlush；有in-flight就通过completion继续，不在main线程轮询/等channel。空白按源恢复；非空走异步Service。完成前允许继续输入时，新snapshot要继续推进gate，flush直到最新dirty清空。
4. 任何失败取消本次关闭意图，保留session和草稿、显示既有错误渠道。一般可恢复错误可由Gate重试；Conflict/Incomplete/closed属于需人工重开或检查的终止条件，不能指数重试同一blocked handle冒充可恢复。
5. 全部成功后在main thread调用Close/Quit重入。第二次listener不是无条件bypass：再次原生最终化并检查实际latest snapshot与已确认版本；如果用户在异步保存期间又输入，继续PreventDefault及新flush。
6. 只有同一main-thread退出回调确认全部最新值durable，才允许本轮关闭。正常回调中不启动nested event loop，故校验到同步destroy之间没有新的native输入插入。App.Quit虽是postMain，重入listener会重新校验，所以post前的迟到输入也不能丢。
7. App quit优先统一flush所有windows；不能依赖逐窗OnClose，后窗失败时前窗可能已被关闭。coordinator共享quit intent但各window/session保持身份。成功才让OnBeforeQuit通过。
8. OnQuit仅做已经允许关闭后的资源清理；Service.Close保持main退出defer或显式最后阶段，不早于所有保存completion。

正常quit无超时后强行放行。M/app.go:197–217的stopGrace在prepareQuit成功以后才启动，与取消quit等待保存不冲突。Force-kill/power-loss不能由该流程承诺不丢未durable文本，不应把no-timeout说成可以抵抗进程强杀。

**当前不能开放第2步的准确实现**；纯Gate通过、链级事务通过或窗口已重绘都不是第2步的替代证据。

## 9. 正式自动化验收矩阵

### UI + 真实Service + 重开（公开Tester，不是截图假数据）

- Type非空，Advance699ms disk不变；再1ms成功后snapshot与重新Open的标题一致。
- 连续输入重置700ms；旧schedule醒来无写入；完全相同和canonical等价输入不额外revision。
- Compose开始/更新/空preedit/commit、替换选区、中文/Emoji/组合字符；composition期间不调用持久化。Enter真实插入换行。
- 空白保持本地；FocusLeft恢复最后成功值；失败后再空白应等待在途结果后恢复准确ack。
- 延迟service barrier：第A保存未完成继续输入B，A回调不得覆盖B，最终只正确确认最新B；不同owner、同owner新generation、旧completion乱序、旧registration注销不得串线。
- selection/date/page/rail/返回统计/完成任务全部实际点击；重新打开原owner草稿不丢、不会写目标任务。
- 自然日AdvanceTo与save并发：两种线性化顺序都合法，原trace过去后拒绝；旧历史trace与同链当前pending反例必须拒绝且磁盘/内存不变。
- 每个disk fault阶段都通过真实保存入口进入：before-reserve可保持旧值；after-reserve/before-commit重开fail-closed；after-persist原handle冻结、内存旧、重开核实新值。UI只在完整durable成功才显示已保存，不自动篡改/修复会话。
- 完成/过去/锁日只读；直接调用Service也不能绕过；旧原型20场景仍是独立视觉回归，不混入runtime事实。

### 必须补的原生缝隙与生命周期门禁

- 保留本次frame前direct client红色测试；修复后同样期望全文，不能换成先tt.Frame再Close。
- 选区replacement、composition取消、native end-before-commit、partial commit、undo/redo、多pass、未建帧就关闭、移除、控件复用、old client迟到。
- 取消close/quit后窗口仍活着；durable pending时loop可继续处理输入；失败后不销毁；成功只关闭一次；二次quit期间新输入重新flush。
- 多窗口App quit：最后一个保存失败时所有需要保留的输入仍可恢复；不要只测window close。
- occluded/最小化窗口退出不会永远等frame；主线程和worker不得互等。用可控channels／dispatch队列，race detector；不以真实sleep认定顺序。
- 原生平台macOS/Windows/Linux IME真实候选选择、立即Cmd-Q/Alt-F4/系统quit及重开对账。未具备环境时明确未运行，不能skip成通过。

### 回滚与放行

本阶段只提交研究/失败门禁与被批准的trace service前置项，UI维持只读。最小框架修复以隔离副本/正式授权补丁实现，依MyGo AGENTS完成各平台编译、原生客户端/IME测试、binary size门禁后才提升dependency版本。再落地adapter并运行上述真实用户路径。无生产cutover、无store格式变更、无旧库迁移。所有文本和错误日志仅使用合成fixture，不在诊断里记录用户标题。
