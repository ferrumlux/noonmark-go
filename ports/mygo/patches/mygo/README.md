# MyGo 标题输入诊断候选补丁

基线：egoist/mygo `977ea57e0baf78da62698ba7371420d192260466`。原依赖的 MIT 许可随 UPSTREAM-LICENSE 保留，Noonmark 应用继续 AGPL-3.0-only。

`0001-preserve-preedit-before-focus-clear.patch` **尚未由正式构建启用**。它仅是 GO-RUNTIME-007 的最小根因候选：失焦的可编辑 string 控件先 commitCompose，再处理 nativeDirty 交付绑定，不能先清掉 preedit 再等帧末 release。disabled/readonly 分支未改，原参考框架与 module cache 未修改。

原生 client 的已提交文本、追加 marked、选区替换 marked 三个原始探针由红转绿，迟到旧 client 仍被当时焦点拒绝。候选 vet 通过。完整 ui race 跑完（344.850s），唯一输出失败为 TestDiffFrameAllocs；本环境未修改基线 targeted race 也失败，steady/within均3/9。普通模式基线与候选均2/7失败。没有删除失败测试、提高阈值或称全套已绿。

此补丁不提供帧外最终文本 API，也没有完整解决遮挡窗口、控件移除、重新聚焦后的旧 client 租约、平台 IME 和退出协调。因此不能只应用本补丁就开放标题编辑，正式 UI 继续只读。稳定 Handle 最终化接口需独立实现、审阅及实机补证。

在任何正式依赖接入前，必须选择明确的可复现下游依赖方案，核对源版本和补丁 digest，跑原始症状、框架相关文本/焦点/Undo/生命周期回归与应用整套门禁。不能静默修改缓存，也不能将本未启用候选当成已经修复的发行版本。
