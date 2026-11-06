# 年度 R2 独立审查

代码冻结清单 SHA d7cd5df7857e14168d797e09894d18a16dbe37693962ed6604b137c0ba188a3e，四项均验证一致。独立 sibling carryover-year-r2-independent-review。只读比较并用精确字符串变换核实，年度 R1 的所有业务断言逐字保留；仅三个 imports 和 fixture 启动块被替换。

## 生命周期合同

先用当前 GOROOT 的 go test -c 编译新路径，再直接执行 fixture test binary。共用父 deadline 减 15 秒或最大 8 分钟预算；不足预算直接失败。fixture 自身 test.timeout 比外部取消早 2 秒。CommandContext 在取消时终止直接 child，Run 等待并记录 startedPID/waitedPID，WaitDelay 2 秒限制输出管道等待。构建失败/取消/预算不足均不会执行 fixture；这部分为静态分支审查。真实 session 只由直接 fixture child 打开，之后才 taskapp.Open。

不宣称取消 go driver 可杀整个 compiler 后代树。没有实际继承输出管道后代测试来触发 WaitDelay，不将其作为动态覆盖。helper 正常/失败/取消/deadline/预取消测试实际启动当前 test binary；取消前确认 child 写入、取消后 3 秒内 Wait 回收，再确认没有晚写。它证明直接 child 合同，不是 OS UI 或全进程树合同。

作者独立 legacy mutation 只把 CommandContext 改回 Command，cancel 用例在 3 秒回收断言变红，并最终等自身 30 秒 test timeout 后 reap。已读取原红日志，未改冻结测试断言。

## 执行

独立快速 process race 会话 97870，日志 process-race.log/.exit；结果完成后补记。年度最终独立 race 等作者会话 63655 完成后串行执行，避免重复重型并发。R1 年度业务与 fixture race 的独立结果仍保留在相邻 year-independent BUILD，不冒充 R2 终态。

快速 process race 会话 97870 已 exit 0，见 process-race.log/.exit。全部预算、成功、失败、取消、deadline、预取消测试通过，无 skip。等待最终年度独立运行前不作完整 R2 acceptance。

## 最终结论：R2 accepted，可进入正式组合整合

独立最终年度 root race 会话 92892 已退出 0：测试 185.92s，包 186.955s，无 skip。普通 fixture 9.85s，真实直接 child PID173=waitedPID173。长滚动 Y1603→660、日期条固定、两次提交各唯一、762→764/revision2→4、历史和无关资料不变、Close/Open 相等全部通过。执行前后四项代码 hash 一致。

已实际查看本轮独立 year-07-reopened-future-2x.png：未来11月7日三项任务，选中“今日长滚动任务32”与右侧标题/日期一致，无明显错位。仍为 headless native Tester，不冒称真实桌面操作或 Swift parity。

精确整合清单 R2_INTEGRATE_SHA256SUMS = f14b8462a23f8431b0a3ca437373e9761fbb53670d12b257e2174970a8911504，作者七项完整文件均逐项核验。YEAR_PROCESS_GATE_ADDITION.tsv 是四个实际 tab 分隔单行，只追加，不覆盖正式 gates。case 状态在整合阶段整合完整 check 与 commit 后更新。本审查未改正式/作者源码，未做 Git 写入。

本轮独立 quick process race 7.215s、年度 race186.955s；内部fixture字节仍与R1相同，已有独立48.496s和整合阶段R2 49.499s证据，不重复该项。整合阶段R2 vet0及后续正式组合aggregate另记，不冒充本独立执行。

本结论覆盖年度测试设施与当前普通任务年度中段业务回归，可与已 accepted 的 pending R1 组合。不是全业务 parity、发版、逐365日UI操作或通用进程树终止保证。无在途测试会话。

本报告保留整合前的独立验证过程；随后正式组合验收见同目录 VERIFY.md，不能把本报告历史时点当作当前未整合状态。
