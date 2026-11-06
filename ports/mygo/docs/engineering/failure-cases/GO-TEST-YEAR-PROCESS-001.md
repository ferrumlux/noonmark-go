# GO-TEST-YEAR-PROCESS-001：年度测试 fixture 子进程生命周期

状态：已修复（仅本地已验收范围：仅fixture直接子进程的预算、取消与Wait回收；不是整个compiler后代树）。实现提交 `f1291ebe1033c5ad37a7ea3b43a838ca6046238c`；无新hosted CI、实机或发行验收。

首次发现：2026-11-06。影响为新年度合成验证设施，不是用户业务操作或生产资料。独立R1以exec.Command执行go test；父测试默认超时和子测试默认超时相同，父退出不能保证驱动及其测试后代及时结束。原R1业务race通过并不覆盖这个退出边界，因此暂缓正式合入。

引入提交：未正式提交的年度R1候选，不存在可确认的引入commit；实际自然人身份未知，不依据Git身份猜测。源码由独立5文件冻结清单定位；正式7c239df未包含年度测试。

根因修正：先在本轮全新外置目录有界编译fixture测试二进制，再以同一Context预算直接执行该二进制。真正拥有development session的进程是直接child；正常、失败和取消均等待Wait后才能返回。父deadline留15秒，未设置deadline时上限8分钟；内部fixture timeout早2秒，WaitDelay为2秒；预算不足或构建失败不得继续启动fixture，不提高root默认10分钟。

自动门禁：fast TestCarryoverYearProcessBudget；symptom TestCarryoverYearDirectProcessLifecycle。真实helper先证明确已运行及写入，再取消或等deadline，验证直接PID已Wait、取消后3秒内收束及无晚写；预取消不得启动。回退探针仅将CommandContext换回Command，保留所有断言，用于证明取消门禁能捕获该类缺口。该探针不是运行完整R1父超时场景，不称为Swift或业务红例。

边界：CommandContext/WaitDelay不提供通用跨平台进程树终止。编译driver的临时compiler后代不打开业务session，其全树终止未验证。继承管道后代的WaitDelay路径与编译失败后不启动fixture目前分别由标准库合同和静态控制流审查支撑，未冒称已有动态覆盖。现有真实helper只验证直接child；所有probe目录均新建合成资料。

回滚：只撤测试设施，不改业务事实或生产状态。R1原代码/日志全部保留；未做GitHub写入或发版。修复状态须在最终组合聚合门禁及精确commit存在后更新。

本轮证据：process-final-race.log退出0（7.182s），process-legacy-mutation-red.log测试退出1（30.083s）；final-native-race.log退出0（195.516s），final-fixture-race.log退出0（49.499s），final-vet.log为空且退出0。原红探针已等待实际PID回收，无未决子进程；未更改业务断言。

## 正式闭环

实现提交 `f1291ebe1033c5ad37a7ea3b43a838ca6046238c`。在正式组合中，完整 scripts/check、六目标编译、主机构建及实际隔离Linux CLI全部退出0；精确门禁与证据见对应任务 VERIFY.md。所有业务源码及原测试断言在验证期间保持不变。本文先前“待整合/未提交”等描述是候选阶段证据，不代表此处最新状态。修复没有修改生产资料、重试待批Git操作或正式发布。
