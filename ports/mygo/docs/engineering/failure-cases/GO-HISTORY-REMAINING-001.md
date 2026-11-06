# GO-HISTORY-REMAINING-001：旧 proof kind 错误接受新增 typed detail

状态：已修复（未接线 helper 范围）。修复已同步为 eb924a14d395391c5625e61effddaed500ddb6b9，精确同 head 的本地检查及 GitHub MyGo run 42198336290 通过，实际 artifact 与完整日志已核；没有接线、发行或影响真实用户资料。

## 症状与引入来源

首次发现：2026-11-06。独立验收从新的未接线 auditReturnHistoryVNext 成功返回取得 proof，按值复制，仅把 kind 改成 historyCommandComplete，仍携带 returnDetail。valid() 错误返回 true，违反 kind/detail 必须匹配的封闭结果合同。这是 package-private 可伪造组合的验证缺陷，不是外部 API、旧 Service、存储或权限漏洞；前批尚无这三个 detail 的整合不受影响。

引入提交：2d76afbea81a746398c9f4329749bced144806b9（本地候选，父 snapshot 4f11424af6bbe3b82f0831a04fc41780bdf50be5）。Git author／committer 均为 Local Candidate <candidate@localhost>，仅是提交身份；本轮实施 agent 写入可确认，具体自然人操作者未知。没有把父 snapshot 或远端 e35a5d5 归罪为引入版本。

原 history_command_vnext.go SHA256：3d88798a1beac64271f34111a79853e35c369d7350874761a9d6beca981e629e。完整首稿可由引入提交精确恢复，原四 production 与557源码清单已单独保留。

## 原始复现

独立首红 TestRemainingIndependentProofKindClosure 原文：

```text
remaining_independent_acceptance_test.go:96: mismatched complete kind with return detail accepted
--- FAIL: TestRemainingIndependentProofKindClosure (0.00s)
```

独立原始focused.log归档SHA256：b0628bae2beda0b74c8568b47a2dceb37225651892b8274cb0b8bda6fdaf683f。作者不修改独立测试或首红。

作者追加实际red：

`go test ./internal/taskapp -run 'TestHistoryRemainingOldKindDetailClosure|TestHistoryRemainingReturnedProofCannotMasquerade' -count=1`

exit1，0.014s；10个旧kind ×3种新detail均错误通过，真实Return改kind也复现。原输出SHA256：ee645b8d4ecdf6aff8dea51810d2599c821e2dc5d85e30794440c9f9f2a40bcb。

## 根因与修订

原实现只在三个新kind分支检查自身detail非nil、其他detail为nil；旧kind分支只保留prior/after有效性检查，未排除新增detail。错误来自实施时过度将“旧分支逐字冻结”当作允许错配的理由。core重放、完整inspector、event builder和snapshot seal均不是根因，不能通过改它们来掩盖此缺口。

根本修复：旧kind分支同时要求三个新增detail全nil。前批真实成功出口永不设置这些字段，所以旧可生成proof语义不变；拒绝的是本批引入后新出现的内部错配。必须明确修订valid旧分支字节冻结要求，而不是虚称旧body未变。不增加任何豁免、开放union或caller callback。

原独立设计reviewer已Accepted最小澄清。修订只给旧kind分支加入三新detail全nil；注册文件修订后SHA256为90e22cf119936f446913e9441b0e4cbbedd02ba97d9da9d79917b72798a92363，其他三production原SHA不变。上述两作者门禁已实际exit0（0.014s），独立原七测试已原封不动换入修订 production，focused/race 均通过；另增独立闭集矩阵也通过。修订候选 taskapp 38.346s、race 246.185s、vet 均 exit0；独立实现审查 Accepted。同一 ed200ac5 共同基线已整合作者与独立测试，四模块 scripts/check、六目标编译与隔离CLI全部退出0；原始固定564文件的SHA/mode在门禁前后完全一致。正式修复 commit 为 eb924a14d395391c5625e61effddaed500ddb6b9；MyGo run 42198336290 / job 127827115063 于2026-11-06 10:18:11 UTC成功。artifact13493154729实际6212字节，SHA256 7dd5548585361e7ca3c27056ce5db9cb027c93486c9a019f5618f61a0d16c83c与metadata和上传日志一致。完整888行job日志及四模块race/vet、六目标编译、隔离CLI核验通过，详见任务目录HOSTED-CI-42198336290.json；不代表跨OS实机或发行。此前全包41.029s、race253.030s、vet通过只证明未含该回归的旧测试通过，不能作为闭集正确性或修订后验收证据。

## 实际门禁与边界

- fast：TestHistoryRemainingOldKindDetailClosure，枚举全部旧kind与三新增detail，并保留无新增detail的旧kind有效性行为。
- symptom：TestHistoryRemainingReturnedProofCannotMasquerade，走受影响pure helper真实成功路径后仅改kind，验证拒绝。不是UI/Service/持久化测试。
- aggregate：scripts/check 的根模块 go test -p=1 -race -buildvcs=false ./... 实际包含两门禁；TestFailureGateRegistry验证登记和函数存在。新增登记后，source scripts/environment准备已核依赖，再实际运行 go test -p=1 -buildvcs=false . -run '^TestFailureGateRegistry$' -count=1，exit0（0.076s）。没有只靠文本登记声称已接入。

没有发行或用户数据迁移，不设置虚构release gate。回退为撤销本批未接线helper与新增注册，保留前批subtask snapshot；不删除或迁移运行资料。今后扩展封闭kind/detail时必须同时检查新kind和旧kind的交叉乘积，不只验证新分支互错。

## 整合期测试代码 vet 失败（非本案 production 根因）

首次共同基线 scripts/check 中，root/UI race528.958秒和taskapp race245.906秒通过后，既有 go vet 门禁发现独立验收文件第73、82、332行的 core.LocalDate{2026, 10, 31} 使用跨包无字段名字面量，整体退出1。原测试仅跑过 focused/race，其报告未声称 vet通过。独立只读复核确认LocalDate定义顺序为Year/Month/Day，仅改为具名字段即值语义等价，未改production、测试断言或关闭composites检查。整合版测试SHA31c5845fb478ce9de92285266ce6a5815fd11b02ffb06f29f25fa769a4ee709a；原独测SHA6036dba28094dd0d8d7622e42b0de2d5d21646a1a2575ae9371a33eefb136610及红日志保留。修订后相同go vet、独立focused与完整scripts/check均退出0。标准go vet继续由scripts/check强制执行，不能因测试来自独立验收而跳过聚合门禁。
