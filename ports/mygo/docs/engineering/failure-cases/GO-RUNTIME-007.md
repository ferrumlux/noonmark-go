# GO-RUNTIME-007：标题退出排空会漏掉替换选区的 marked text

状态：处理中。原始依赖反例保留；downstream 框架修正已通过支持入口的原始反例、定向回归与应用聚合验证。后续日标题编辑已接入隔离 development 应用，真实 Handle 的帧前输入、导航和退出症状已有 headless 回归；真实操作系统输入法及系统退出仍待验收。下方“只读／未开放”是首次发现阶段的历史处置，不代表当前接线状态，也不宣称实际用户资料发生过丢失。

- 首次发现：2026-11-05，connect-title-autosave 依赖接线研究。MyGo 锁定版本为 `977ea57e0baf78da62698ba7371420d192260466`；未定位引入此行为的精确上游 commit 或实际修改者，未知，不能把整个依赖作者当成故障责任人。移植仓库基线 `986cd8d` 尚无标题编辑 UI。
- 原始症状：原生 TextInputClient.ReplaceText 在帧前已修改 editor，但绑定 draft 仍为旧值，ObserveInput 不会收到这条原生路径。直接在 OnClose/OnBeforeQuit 保存 draft 会漏最后输入。
- 更强反例：全选旧标题后原生 SetMarkedText 替换选区，未发生新帧就要求退出。保留 editor、root.Focus、后续 pass 收集 draft、Disabled 的两阶段方案仍得到空字符串而不是“未定”。没有真实用户资料，所有文本为最小合成哨兵。
- 定位证据：ui/runtime.go 每帧可重建三个 pass；ui/value_input.go 的 applyString 在失焦时直接清 ed.compose，帧末 ui/input.go:updateTextInput 才 old.release/UnmarkText，已无法提交清掉的 marked 内容。直接移除 composing 控件也不会产生可用的 FocusLeft flush。
- 已排除：仅 append marked text 与普通已提交 ReplaceText 的探针通过，不能据此证明选区替换安全；ObserveInput 计数不能覆盖原生 client；固定等待时间或固定帧数不能作为正确性屏障。
- 可执行红色门禁：`scripts/probe-title-native-input`，将锁定 MyGo 精确源码复制到外置临时构建目录，再用 Go overlay 在原生 client 路径注入最小测试（Go 禁止 overlay 直接替换 module cache），不改参考源码或 module cache。预期 `marked-selection-before-frame` 失败。该门禁是标题 UI 激活前的阻断，不混入当前未启用 UI 的绿色 scripts/check，也不把预期失败改为 pass。
- 最小测试保存在 `docs/tasks/connect-title-autosave/probes/native_input_test.go.txt`。后续修复必须原样令该症状转绿，再测取消、替换范围、组合选择、Undo、重复关闭、迟到 client、跨窗口与 app quit，不只测试追加输入。
- 当前处置：保留只读详情；先完成纯 gate 与同锁 trace 路由。框架修复/API 和 native finalize 契约须独立验证，不用新 Save 按钮、sleep 或强行清空 composition 绕过。没有生产发布、没有存储迁移；旧版本无需清理资料。

## downstream 最终化边界

支持入口以官方977ea57e锁定ZIP加hash校验的小型patch使用外置副本，原module cache不变。新增Handle.TextInputSnapshot/FinishTextEditing只支持UI线程帧间内建string控件，排空已收到输入、提交preedit、同步binding、撤销旧client；旧client再聚焦也无效。明确拒绝失效/隐藏/关闭身份、TextBuffer/custom client及只读/禁用Finish。

37项定向回归覆盖原始选区探针、Undo、重入关闭/host修改同一binding、readonly/disabled、帧间/遮挡不绘制、迟到client、frame-local binding等。原始红探针脚本显式GOWORK=off仍测试原官方来源；scripts/check-mygo-native测试受审副本，scripts/check调用此门禁。

隔离完整ui race只有原版同样失败的TestDiffFrameAllocs（steady3/within9），未改阈值，不能声称框架全绿。框架边界修正本身不证明应用Close/BeforeQuit已接通；后续实际接线与独立症状门禁见 `../../tasks/connect-title-autosave/NATIVE-UI.md`。真实三平台IME与Swift动态oracle仍待验收。

## 应用路径门禁

`TestTitleUICloseBeforeFrameAndInitialRegistration` 使用实际 MyGo Handle 和原生 client，在未建下一帧时提交普通文字或替换 marked 选区，再经过真实应用 guard 和合成持久化。`TestTitleUIRepeatedCloseAndLateInput` 验证已在途请求、迟到输入及再次关闭；`TestTitleUIMultiwindowQuitFailureKeepsEveryWindow` 验证失败不销毁任何受影响输入。它们由 `scripts/check` 强制运行，不把 public Tester 自动建帧当作帧前输入证据。原版红探针继续独立保留，不改成虚假的成功门禁。
