# 分类显示名相等性修复验收

独立候选基于正式 `a91eba40ea0ddb20fe3467127f25f88e7c3166aa`，不改原Swift、原型或最初三批模块，不触碰生产。仅修正低层分类引擎rename条件，不开放任何非空catalog运行入口。

- 修改前6个规范等价分支实际红，原日志外置 `noonmark-classification-equality-build/evidence/red.log`。
- 同一测试修正后要求实体原字节与nameVersions完整不变、revision不增、一次no-op记录可严格重放、重复interaction返回原receipt且无第二写入。
- 8个同fold-key但不canonically equivalent分支须继续真实rename，包括实际CGJ边界，避免用key相等代替显示名相等。
- 旧snapshot通过尚未修正的正式engine实际生成，版本/records均保留；新reader须拒绝错误历史且不改写输入/文件。fixture只用于明确兼容边界，不是迁移器。
- fixture名称表明确synthetic-only；现有严格非空catalog拒绝及所有既有分类测试必须继续通过。

源码/红绿日志和完整测试结果外置保存。Swift oracle、真实分类UI、非空catalog持久化和六平台原生运行均不在此修复的验证范围内。

最终候选 scripts/check、六目标cross-build、Linux development CLI均exit0。应用main race55.884s、classification1.197s、native ui2.521s/root1.044s；51项依赖门禁通过。完整证据为外置evidence/{check,cross,entry}.log。另实测旧正式reader可接受原legacy fixture；新reader拒绝不是手工构造坏JSON造成。独立复核也已完成四模块 race/vet、51 项依赖门禁及 native ui/root gates，全部 exit 0；独立恢复旧 engine 再次复现 6 个红例，并核实旧 reader 接受原 fixture。正式整合也已重新运行 scripts/check、六目标编译与 Linux development CLI，全部 exit 0（main race55.368s，classification1.196s，native ui2.605s/root1.047s）。证据外置 evidence/integrated-{check,cross,entry}.log。尚未把本地结果当新远端 head 的 CI 通过。
