# GO-SERVICE-HEALTH-001：已发现的 prefix 异常可被旧入口遗忘

状态：已修复。正式修复 commit cf9a77f8015f4dcf579e23c582fa53385091a7a6 已同步；精确该提交 hosted CI run 42207724372 的完整日志和真实产物验证通过。手动滑杆与恢复关闭仍未启用。

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

## Hosted 闭环

2026-11-06 11:25:57 UTC，精确 cf9a77f8 的 [run 42207724372](https://github.com/ferrumlux/noonmark-go/actions/runs/42207724372) 全部10步成功。artifact 13498821489 实际下载6198 bytes，SHA256 1a3d9d83bc3a3b7bae6d6b300b62dc62d69b569f8e605d5e917f66b0da1beab8，与GitHub digest及上传日志一致。六文本CRC通过，三个执行报告全文按序对应完整888行日志。结构化证据见 ../../tasks/service-typed-outcome/HOSTED-CI-42207724372.json。

本次证明四模块race/vet、Unicode16、固定依赖定向native门禁、六目标编译和Linux合成headless/CLI通过；应用日志是包级，非逐项Service测试列表。没有本次Swift执行、非Linux实机、桌面IME、完整框架allocation或正式发行结论。
