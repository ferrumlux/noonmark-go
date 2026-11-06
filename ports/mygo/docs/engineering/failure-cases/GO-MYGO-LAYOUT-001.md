# GO-MYGO-LAYOUT-001：Selected 字段漏登记布局输入

状态：已修复（精确候选feab6d94；整合至293b22后完整scripts/check退出0）。独立候选及新组合证据见 docs/tasks/repair-selected-layout/VERIFY.md、INTEGRATION-VALIDATION.json；同步后hosted CI已由bff7414的run 42240698683补齐。仅处理默认补丁链的测试登记及门禁，不改生产运行语义，不启用 native recovery 研究补丁。

## 可核对的来源

本地原始 Git 历史的 0fcf95ecc4a134163d7df21215578daa555be89a 首次新增 0005-selected-access.patch 和 0006；其父提交仅有0001–0004。0005 的 SHA256 为 ab7aa8f5be93e4260eddff35728c86792a95d8a16f0b11dd56a396c10a09966a，与当前文件相同。该提交加入 node.selected，却未更新上游已有 TestLayoutInputs 的字段登记。Git author/committer 为 Codex <codex@openai.com>，这只是提交元数据，不能证明实际操作者身份。

当前备份分支不是上述原始历史的后代。其 c2ca275d8e43975b82f71a633decfe6d0dc88b97 同步已验证的本地快照，是此文件在当前备份历史的首次出现，不能误称其为原始缺陷的创建提交。更早缺陷不存在于固定官方 archive 的该字段，因为 node.selected 是下游0005新增。

## 首红、根因与修复边界

官方固定 archive 经默认0001–0006及前后hash验证后，单跑 TestLayoutInputs 报 node.selected 未分类。它是反射登记失败，不是几何差异证据。直接读写与 selectionOn 调用链核查：setter写状态及颜色；access生成选择状态；outline/list/tree读颜色或hover；runtime用于focus group。layoutcache/sameLayout和布局不读取 selected；颜色不影响 textLayoutSet。故登记为 other。

追加0007-selected-layout-regression-tests.patch，仅改 ui/layoutcache_test.go 并新增 ui/noonmark_selected_layout_test.go。原六补丁及所有生产文件保持相同字节。新症状测试对 Button、Tab、TreeItem、Row、ListItem 真正调用 Selected(false→true→false→true→false)，不覆盖其颜色副作用；cached计算次数不增，对照禁用cache确实计算，逐节点几何/文本布局相同，整个access tree相同且每次状态符合期望。选择角色原有 AccessChecked wire alias 保留，不能错误要求与普通按钮相同。

## 预防门禁

scripts/check-mygo-native 默认加入 TestLayoutInputs 与全部 TestLayoutCache*；原 TestNoonmark* 自动覆盖新症状测试，保留 --all-ui、race、vet、host popup检查。scripts/check仍调用此入口。根包 TestSelectedLayoutGateContract 执行真实默认及聚合脚本，使用记录Go实参的stub核对可达命令、顺序、筛选、race/all-ui及首个失败立即传播。TestSelectedLayoutDependencySymptoms 真正执行准备后的依赖go test -json，精确核对TestLayoutInputs、新症状及五个角色子测试均run/pass且无skip，拒绝零匹配假绿。两者登记于gates.tsv。TestSelectedLayoutPatchScope仅作辅助静态范围检查，不当症状门禁。仍须另跑完整ui race。

不涉及生产资料、用户迁移、remote、原生OS实机或Swift对等。回滚只撤此独立测试补丁及门禁增量，不撤已存在的Selected语义。默认完整UI失败不能再被筛选成功冒充。

## 精确 hosted CI 收口（2026-11-06）

[run 42240698683](https://github.com/ferrumlux/noonmark-go/actions/runs/42240698683) attempt 1 于15:33:28Z成功完成。正式head为bff7414d71c10f31f45c679393e035aa6386b989；本地候选0b33571a1d3892201d9c457051db25c6a45eaabb与它的tree均为04482b01e72640b808d7d49c65b7acf728fadfc3，但两者不是同一commit。此证据不改绑到后续文档／测试提交。

原默认600s根包race在492.032s完成，余107.968s；四模块race/vet、Unicode16、51项依赖合同、默认native及host popup、六目标编译、Linux合成CLI全部通过。TestLayoutInputs、TestNoonmarkSelectedLayoutCache及其五角色子测试明确PASS，另两项Selected状态／顺序测试也PASS。根应用测试为包级非verbose输出，不声称逐项名称都单独记录。

公开精简回执：docs/tasks/diagnose-root-test-timeout/HOSTED-CI-42240698683.json，记录来源、job／artifact身份、摘要和计数。6435字节artifact的SHA256为8eb73ce4212ca68ec6aaa514499e5a9e9dfef48ca3055ba877aeded25f50e1c8，与GitHub及上传日志一致；六个安全成员CRC通过，阶段文本与完整job日志对应。

仅关闭GO-PERF-DAY-001的本次精确head超时缺口及GO-MYGO-LAYOUT-001的对应hosted证据待项，不改变其它案例。单次600s内成功不保证未来runner性能；完整framework allocation／TestDiffFrameAllocs、ICU76、真实桌面／IME、六平台runtime、新Swift、签名／release和完整产品对等均未获此证据验收。产品源码、fixture、默认补丁、冻结清单和许可证不变。
