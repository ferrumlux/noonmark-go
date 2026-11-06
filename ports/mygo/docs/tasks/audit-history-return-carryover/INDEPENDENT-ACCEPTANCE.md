# Closed proof 修订独立复验

结论：**指定修订通过本次独立定向复验，原封闭合同缺陷已红转绿。** 不是完整矩阵、全包回归或全产品接受。

## 最小结果

- 保留原 remaining-history-independent-behavior 与 remaining-history-independent-build 全部原 production、red、报告，不覆盖或改写。
- 新隔离 clone：remaining-history-independent-revision，仍 checkout 本地 snapshot 4f11424af6bbe3b82f0831a04fc41780bdf50be5，再只覆盖四 production。来源候选读取 HEAD 为32befa59455f74078aeded954ba96578b2092fdb，不是远端提交认证。
- 当前 manifest 四 SHA 全匹配；注册文件为90e22cf119936f446913e9441b0e4cbbedd02ba97d9da9d79917b72798a92363，其余三文件与原 freeze 一致。对原版 diff 只有 valid 的旧 kind 分支增加三新 detail 全 nil，未改其他函数。
- 原独立测试原封不动 SHA6036dba28094dd0d8d7622e42b0de2d5d21646a1a2575ae9371a33eefb136610。
- 原七个顶层独立测试 focused 与 race 均 exit0。原 TestRemainingIndependentProofKindClosure 从两种运行均 red 变为均 green；预算抢序、map ownership、四路径、1545次 direct fence 污染等仍过。
- 另增独立闭集测试 remaining_independent_closure_revision_test.go，SHAa813ecabfbe33db4d21e69eca18baa01de1719ca5896fe1cff7c716c468d6da1，单列 focused/race 均 exit0。没有读作者新增 tests。

## 扩展测试实际边界

额外测试从真实成功 R、D、Continue 三个 proof 出发：各改为十个旧 kind 后全部拒绝；对每个新 kind 遍历八种 detail presence mask，仅匹配单一 detail 放行；缺 seal、无效 prior/after、zero/unknown kind 拒绝。所有拒绝组合同时检查 snapshotCopy、changed 不存在旁路。

另从真实 Complete/Undo 成功 proof 验证新 detail 全 nil，valid/snapshotCopy/changed 正常；再覆盖十个旧 kind 的 nil-detail 注册分支及注入各新 detail 均拒绝。后者跨 kind 是 private validity-shell 测试，不冒称其余八种旧入口均在此扩展中真实执行；未替代父级旧测试回归。

## 命令与证据

显式新建独立 external BUILD，export NOONMARK_MYGO_BUILD_ROOT 后 source 原 build-environment/activate.sh；官方 go1.27.3 linux/amd64，独立 GOCACHE，只共享指定 modulecache。

在新 clone 的 ports/mygo：

1. `$NOONMARK_GO test ./internal/taskapp -run '^TestRemainingIndependent' -count=1 -v` → original-tests-focused.log，exit0。
2. `$NOONMARK_GO test -race ./internal/taskapp -run '^TestRemainingIndependent' -count=1 -v` → original-tests-race.log，exit0。
3. `$NOONMARK_GO test ./internal/taskapp -run '^TestRemainingRevisionClosureMatrix$' -count=1 -v` → extra-closure-focused.log，exit0。
4. `$NOONMARK_GO test -race ./internal/taskapp -run '^TestRemainingRevisionClosureMatrix$' -count=1 -v` → extra-closure-race.log，exit0。

source-sha256.json 绑定本次所有 production 与两个独立测试文件。原七测试和额外闭集测试采用不同名称与日志，未把增加覆盖伪称为原测试完全未变的覆盖。

## 明确未覆盖

仍沿用原 REPORT 的覆盖缺口：未穷举全部 guard/overflow/MaxFloat/负 priority、多流次序、所有 EventID 大小写 alias、revision/canonical capacity、所有资源边界、AST 自动门禁或全包 tests/vet/scripts/check；未执行每种旧 kind 的真实成功入口。预算测试是固定错误先后和有界大 raw/map 负例，不是对每次 Clone/Validate 的调用计数。

未验证 Service/write/head/clock 接线、持久化故障/重开/双handle、v3 clean cut、Swift/macOS/Windows 或 UI；无远端/CI 写入，无 production/home/iCloud/Keychain/live AI 探测。当前正式 ed200ac5 与本次本地 snapshot 隔离来源不同，未混称。本次只确认给定四 production 修订通过这些定向独立测试，不能作为全产品验收。
