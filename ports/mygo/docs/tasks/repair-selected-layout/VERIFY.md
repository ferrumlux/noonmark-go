# Selected 布局登记窄修验证

基线 cf9a77f8015f4dcf579e23c582fa53385091a7a6；Go1.27.3；外置 selected-layout-build，独立GOCACHE。GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB。无研究补丁、远端写入、生产数据或运行语义改动。下列结果是隔离候选的本地证据，整合Day批次后及hosted CI须另验。

## 原红与修复机制

官方固定archive经默认0001–0006及verify_selection重建后，真实 `go test -p=1 -count=1 -buildvcs=false github.com/egoist/mygo/ui -run '^TestLayoutInputs$' -v` 退出1：node.selected未登记。red-layoutinputs.log保留原红。

追加test-only0007，TestLayoutInputs及TestNoonmarkSelectedLayoutCache通过。五角色均真实调用Selected false/true反复切换，不覆盖颜色副作用；缓存不重算，禁用缓存对照确实重算，逐节点几何/文本布局及完整access tree相同，并断言真实选择状态。开发中首次误把选择角色既有AccessChecked wire alias视为错误，保留test-development-wire-alias.log，依据0005兼容契约修正断言，没有改runtime。

原六patch与基线完全同hash；原修复准备依赖只修改ui/layoutcache_test.go、新增ui/noonmark_selected_layout_test.go，其余2195文件相同。原始历史0fcf95e首次加入0005，当前备份分支c2ca275为同步快照；详见GO-MYGO-LAYOUT-001及外置evidence/introduced-source.txt，不推定实际操作者。

## 执行级永久门禁

首候选e9f9d94的静态根门禁被独审拒绝：插入exit 0或把patch全文注释仍可令grep检查通过。首候选及独审反例保留。

2b9376c修订后，真实执行check-mygo-native（default/all-ui/首错）及check（default/首错）脚本，stub记录实际Go实参及调用顺序，首个非零19须立即传递。根症状wrapper真正运行准备后的MyGo `go test -race -count=1 -json`，核对TestLayoutInputs、新症状主测试和五角色子测试均run/pass，拒绝skip/零匹配。静态scope检查仅辅助，不再登记为症状。独审R2 Accepted仅覆盖此窄修及永久门禁，不等于完整UI全绿。

同一修订测试binary的五个外置反例均退出1：native-early-exit、aggregate-early-exit、wrong-filter、commented-native-command、zero-matching-tests。最后一个实际构造无目标测试MyGo module，子go test退出0而wrapper拒绝。对应*.log和evidence/guard-mutations.json保留。

## R2固定身份完整验证

源码提交2b9376c7faf91fb4e4d2d5e10728e10bbd4f0640；manifest SHA256 07b12b58dd1b40087c91186f6a2a4974991488d7e42383819068948fb7233232。

- 默认scripts/check-mygo-native（UI race、vet、host popup）：退出0，default-native.log。
- scripts/check-dependency：51项通过，包括manifest/patch/workspace负向及真实Go选择，dependency.log。
- 根Selected/registry专项含真实依赖wrapper：退出0，root-focused.log。
- 六目标cross-build：linux/windows/darwin各amd64/arm64，全部退出0；只证明编译，cross-build.log。
- 首次完整scripts/check：2026-11-06 12:08:30Z–12:25:56Z，退出0；根包race539.997s、taskapp261.184s，其余包、各模块vet、canonical16、准备器及默认native全部通过，aggregate.log。因先让Day批次完成，旧等待session过期；先确认从未START aggregate，才启用新runner执行本次首次聚合，没有重跑既有full-ui/cross。
- 完整ui race：1180个顶层测试，422.603s，退出1。唯一失败TestDiffFrameAllocs（steady3、warm9，原要求不超过2）；所有LayoutCache、TestLayoutInputs及新Selected症状通过。full-ui-race.log及evidence/full-ui-summary.json保留。该allocation失败与既有connect-title-autosave/DEPENDENCY.md、GITHUB-CI.md记录一致；没有-short、skip、改阈值，不宣称完整UI全绿。失败后该--all-ui流程未继续vet，默认入口另已真实通过。

## R3仅补丁序列化修正与新身份复验

整合检查发现0007的六行unified context含space+tab，作为Git新增patch文本被diff --check拒绝。red-patch-whitespace.log真实退出2。改为unified=0，保留同一旧/新代码行及完整新测试文件，不增加whitespace属性或忽略开关；green-patch-whitespace.log退出0。

最终patch SHA256：2fe59ea28a4d61362036ec44fccba6df8113a72c338496d334cde6492dcbd6f9。
最终manifest SHA256：a98fe0ac78b3b7b17c758354d9b422ecbf1fdc98fc0753403dba87c64d0b268a。

以新身份从官方archive重新准备并验证选择后，新旧完整依赖2197文件全部逐字同hash，无增删；evidence/r3-byte-equivalence.json记录精确路径和两manifest。原patch/manifest另存r2-0007.patch、r2-manifest.json，旧完整结果没有改绑到新身份。

新manifest真实复验：r3-dependency.log（51项，退出0）、r3-native.log（默认UI race/vet/host popup，退出0）、r3-root-selected.log（根registry/Selected/执行级门禁/真实症状wrapper，4.117s，退出0），2026-11-06 12:30:32Z完成。R3没有重跑完整聚合、完整UI或六目标；这些R2结果仅作为字节相同源码的既有证据，身份区分如上。

完整日志在外置selected-layout-build；本目录validation-receipt.json保存每份日志的SHA256和字节数，source-identity.json保存原六patch及最终源身份。最终只追加测试补丁、测试门禁和说明，不改应用或依赖生产源码。没有原生OS实机、读屏或Swift对等声明。
