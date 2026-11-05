# 锁定原生输入修复的构建来源

日期：2026-11-06（新加坡）。设计先于入口修改：原 go.mod/go.sum 保持官方 MyGo pin；支持的 scripts 先校验官方 ZIP 的 module h1、go.mod h1、archive SHA256、MIT LICENSE，再应用逐文件 before/after hash 与零 fuzz/offset 的小型文本 patch。外置不可变副本由四-use + 精确版本 workspace replace 使用，不把 MyGo 扩为 main module，不改 module cache 或原仓库。

本批新增帧间 Handle.TextInputSnapshot/FinishTextEditing，复用既有内建 string editor、清理已交付 preedit、同步绑定、撤销旧 native client 租约；不支持 TextBuffer/custom client。必须在拥有窗口的 UI 线程及 build 之间调用，不保留 Element。当前只接可验证构建与薄适配器，不开放可见标题编辑器，不将 FlushClean 等同真实 Close 许可。

支持入口需要 Bash、Python 3.10+、Go 1.27.3；构建脚本已在 Linux验证，macOS开发环境及Windows原生构建脚本未实测。目标程序仍为 macOS/Windows/Linux amd64/arm64，运行时不需要 Python、Bash 或 Bun。新 API依赖精确patch，裸Go命令不再是支持构建入口。

统一environment固定GOENV=off、GOFLAGS=-mod=readonly及外置GOMODCACHE，阻止个人modfile/overlay配置意外改变受审来源。缺官方ZIP时仅通过Go标准module download拉取明确锁定版本，然后做全部hash核验；准备器本身无网络。每次入口重新校验完整产物与receipt，实际go list解析版本与Replace.Dir再验证。失败锁/不完整产物不自动删除；修复输入后可选全新专用build root。

脚本 check-mygo-native 默认执行定向回归，--all-ui 执行完整ui race。probe-title-native-input明确切回未修改官方依赖，保留原始红例，不能拿基线失败冒充修复失败。check-dependency执行合成prepare门禁。

证据分开记录：隔离overlay最终37项定向测试、race/vet、两个非使用样例release文件大小不变；完整ui race338.880s只有TestDiffFrameAllocs失败，steady3/within9与未改基线相同，diff滚动113对基线112。没有改阈值或宣称框架全绿。prepared正式路径与全应用复核结果见下节。真实三平台IME、非Linux实机、Swift oracle和应用退出流程尚未验收。

## 正式入口验证结果

最终scripts/check通过：应用主包race35.107s、taskapp10.664s、session1.335s，四模块race/vet/gofmt通过；新增50项依赖准备门禁全部通过（20.445s），受审prepared MyGo的37项定向原生回归与vet通过（race1.878s）。六目标交叉编译、development入口新建/重开/拒绝复用通过。原官方baseline红例仍准确失败captured=""/want="未定"，修正版相同期望通过；没有改变期望。

证据位于外置build root：evidence/prepared-dependency-{app-check,cross,entrypoints}-final.log、prepared-native-baseline-red.log、prepared-native-check-final.log。新增薄native adapter通过真实Tester帧间验证，build期间拒绝，不使用fallback draft。首次适配器测试拼写了不存在的modifier常量，编译即拒绝；已改为框架实际ui.Cmd并重跑聚合门禁，未降低断言。

API生成核对也通过：隔离源码运行go generate ./ui生成字节与官方完全相同；internal/uigen与internal/uimigrate（含TestUIAPI）测试通过，内部go list确实加载新Handle契约。新方法不属于node/context生成facade，无需修改api_gen.go/uiapi.go。UI线程限制沿用Handle/Context现有契约，并非mygo主包任意goroutine调用承诺。
