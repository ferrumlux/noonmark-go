# GO-SERVICE-HEALTH-001：已发现的 prefix 异常可被旧入口遗忘

状态：处理中。本地候选修复commit 71b81f01951b9a8cada87fa3fa4c2612f6c53b60 的完整聚合、独立盲测及源码审查通过；共同基线已整合独立测试并完成完整检查、六目标编译及隔离CLI，修复提交和对应hosted CI仍待同步核验。未启用手动滑杆或恢复关闭。

## 症状、证据与来源

2026-11-06 在正式基线 eb924a14d395391c5625e61effddaed500ddb6b9 的隔离副本上，先由真实领域命令建立 development session；篡改 COMMITTED，使 legacy SaveTraceTitleInputAcknowledged 的 no-op 返回原 ErrCorrupt；外部恢复该文件原字节后，legacy AddToday 竟然成功。没有新增 typed API 的编译错误充当产品缺陷。

首红 TestOutcomeLegacyPrefixFailureCannotRevive：

```
known prefix failure was forgotten; subsequent legacy AddToday = <nil>, want ErrIncomplete
--- FAIL: TestOutcomeLegacyPrefixFailureCannotRevive (0.03s)
```

原始日志保存在外置 service-outcome-build/test-contract/initial-red.log，exit 1，SHA256 88a9cc0705385a9c14a34b9811f3c7f65af271ceb0b3ddfa593e296e4c8802ef。完整源及函数manifest在同build的evidence。git blame将原title no-op的直接verifyPrefix行定位至e5dd046c，但这不是完整引入回归的二分证明，不归因给自然人。

## 根因与修正

verifyPrefix是纯读校验，失败不一定设置disk.blocked；旧Service没有粘性健康记录，transact甚至只检查closed。修正由Service在同锁内保存有界失效原因，commitLocked/verifyPrefixLocked合并结构化阶段观察；所有旧写入口也必须先经gateLocked。外部恢复字节、清除旧disk.blocked或读取快照都不能恢复旧对象的可写性。

普通领域拒绝、候选容量不足和before-reserve故障仍可Writable。当前调用Rejected/None并不证明此前revision没有写入；已知prefix异常仍须NeedsReopen。首次错误原样返回，之后统一ErrIncomplete。成功中的临时blocked不污染Service。

## 永久门禁

- 快速：TestOutcomeProtocolRejectsInvalidCombinations。
- 症状：TestOutcomeLegacyPrefixFailureCannotRevive；覆盖真实旧API忘记错误的原症状。
- 扩展：TestOutcomeLegacyAllMutatorsRespectStickyGate、TestOutcomeRawDiskOwnership、TestOutcomeExactIOFailures、TestOutcomeLimitReasonIsNotErrorName以及两阶段和并发测试。
- 聚合：scripts/check原样保留，TestFailureGateRegistry验证登记。作者固定71b81f0全scripts/check通过；正式eb共同基线整合后，固定577文件SHA/mode前后全匹配，root/UI race524.419秒、taskapp race259.443秒以及其余模块、vet、Unicode、依赖/native全部通过。六目标编译和Linux隔离CLI创建/重开/复用守卫通过。独立10测试整合race3.720秒，作者专项及冻结门禁race6.261秒，登记gate race1.507秒；均实际退出0，不以静态登记代替执行。

不更改core/codec/磁盘wire、fault四时点、提交确认顺序、vNext/v3、UI/hub/coordinator。回退只撤销此未发布代码批次，不删除/修复/重写任何失败会话资料。旧对象关闭不是UI RecoveryClose或恢复文件协议。
