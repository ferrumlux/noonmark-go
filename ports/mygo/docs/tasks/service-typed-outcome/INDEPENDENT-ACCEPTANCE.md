# 独立行为验收

生产来源：基线 eb924a14d395391c5625e61effddaed500ddb6b9 加冻结候选 71b81f01951b9a8cada87fa3fa4c2612f6c53b60 的仅生产 delta。production.patch SHA256 为 200b26301db269f2ca96149fe01b0c13898a3b68e3fa0cd041719dd6ddac96e2，8 个生产文件逐一核对冻结清单通过。未载入作者新增测试、README 或门禁。

先写独立测试 SHA256 e193b62f5728cb5fe965bd6ba34574cf87804b90be42c1c76187a6caa950e967，载入生产后未修改。完整 artifact 为 outcome_independent_test.go 及 independent-tests.patch。

## 覆盖与实际断言

10 个独立顶层测试，以真实 Create/AddToday/Manual/Advance/Continue/Subtask 领域命令建立 development fixture，磁盘位于独立 BUILD 临时目录；未伪造 Service、graph 或 outcome。

- 25 个 legacy mutator 入口在已知 prefix 失效之后，无 wall clock、无图发布、无文件变化；外部恢复原 bytes 不清除粘性会话。
- nil→0 与 100 clamp，100 保持 pending；manual/advance no-op 无 wall clock 或新写入；receipt 不随其他命令或 Close 改变。
- 健康领域拒绝不会冻结；关闭与非法日期错误旧优先级保留，Session 同时准确。
- before-reserve、after-reserve、before-commit、after-persist 四个精确 fault，首次错误、三维结果、无候选发布、后续停止及真实重开结果分别断言。
- 在真实文件系统创建竞争目录、独占 payload/marker 冲突、persist 后 COMMITTED 损坏，验证 reservation possible / reserved-or-partial / written-unacknowledged 的区别。
- 旧 COMMITTED 损坏、身份 marker 变化、此前残缺额外 revision、条目超限，当前命令 Rejected/None 不代表会话健康。
- Health 不主动 scan，不成为命令 receipt；返回值修改不会改变真实健康。
- Advance 已确认后，历史 setter 被正常拒绝，前段 receipt 仍保留且无关命令可写。
- Advance after-persist 与 Close 并发串行化，旧对象不发布未知候选，新 Open 独立核实已落盘事实。
- 真实跨日 Continue 历史 floor37、回退钳制及全链 child guard。

## 未覆盖，不作通过声明

新候选 record/session/revision 硬容量上限、候选 normalize/audit/clock 各种拒绝、OS 短写与文件 Sync/Close 错误、目录 fsync 错误、全部 pathGuard/目录身份/JSON/parent 子分支及所有 legacy 首次检测入口，未逐项注入。条目超限测试仅证明 prefix ErrLimit 会停止，不等于验证候选容量 ErrLimit 可继续。没有修改生产添加测试钩子、提高上限或伪造返回值补足这些空白。

本验收不代表完整 taskapp 或聚合 check 通过。基线冻结 manifest 与获准生产 delta 不一致，未载入作者门禁更新，也未宣称该门禁已通过。未验证 UI、RecoveryClose、原生草稿、跨平台断电 durability 或实际桌面交互。

## 实际运行结果

最终 runner 只选择本 artifact 的 10 个精确顶层测试名称，不依赖基线其他独立测试或作者新增测试。focused、race、vet 三项实际执行，退出码均为 0；详情分别见 focused.log、race.log、vet.log 及对应 .exit。vet 日志为空表示无诊断，不是未运行。初轮宽前缀额外选中基线测试的日志另存 broad-prefix-*.log，不用它替代精确独立证据。

运行后原测试 SHA 保持不变；生产八文件 SHA 再次逐一核对通过。正式 checkout 与远端没有更改。
