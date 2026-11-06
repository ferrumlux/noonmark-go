# GO-HISTORY-SUBTASK-001：实际子任务删除见证的独立护栏误用ID对象

状态：已修复（未接线 helper 范围）。修复已整合并同步为 ed200ac5d4e269fb23c884b3f19d817f928e8d87，同 head 本地聚合、MyGo hosted CI 与产物核验通过；未发行或接入 Service/UI。

## 症状、发现与来源

首次发现：2026-11-06（新加坡）。在未接线typed子任务历史helper候选中，合法actual delete经core replay及完整final inspector成功，随后独立canonical fence误拒`frozen facts changed`。影响是新的纯audit不能接受合法删除边；旧Service/store/UI/v2 writer未接线、未修改，未影响真实用户资料。

引入版本：基于`c2ca275d8e43975b82f71a633decfe6d0dc88b97`的本轮尚未提交候选。不存在引入commit，不能把基线提交归罪为引入版本。Git author／committer不适用；可确认本轮实施agent写入首稿，具体自然人操作者未知。错误首稿完整源码快照及hash未保存，未知；不把从修正版本反推的文件冒充当时原件。

首红来自实施团队执行的独立字段oracle和另一测试worker编写的专属fence测试，均实际运行，不是静态推断。原执行命令为：

`go test -count=1 ./internal/taskapp -run '^TestHistorySubtask'`

首轮完整输出归档名`GREEN-FIRST.log`（名称不代表通过），SHA256为`b1df536420ad8af500735ed179f3e64d5cddc96f4c2f871d54a62061ffadfb2b`，exit 1。以下为该输出的原文相关摘录，保留于此以免只依赖外置目录：

```text
--- FAIL: TestHistorySubtaskActualIndependentOracle (0.04s)
    --- FAIL: TestHistorySubtaskActualIndependentOracle/delete (0.01s)
        history_subtask_command_vnext_test.go:176: candidate history command: frozen facts changed
--- FAIL: TestHistorySubtaskActualIndependentFence (0.04s)
    --- FAIL: TestHistorySubtaskActualIndependentFence/delete (0.00s)
        history_subtask_fence_test.go:130: legal delta rejected
```

同轮其余fixture错误另按原core约束修正，不能以它们掩盖这个真实production缺陷。

## 根因、排除与修复

`domain-codec/v7/schema.go`把subtask的`draftCancellationID?`定义为`uuid`，`domain.go`编码为普通字符串。首稿`historyActualSubtaskFence`错误使用`historySubtaskID(string(in.CancellationID))`，生成`{"rawValue": ...}`对象。真实core replay与bridge输出正确，两者因此不相等。parent/child clocks、ID分配、全图inspector或取消语义并非这一错误的原因。

修复只把delete分支中固定期望的`draftCancellationID`改为`historyString(string(in.CancellationID))`。没有改schema、core、profile或旧writer，没有把字段加入豁免、放宽全图equality，也没有删掉fence。ID/lineage/trace等真正`ID`类型仍保留其wrapper；actual delete继续保留身份与位置、清CompletedAt、写X／SettledAt／UpdatedAt，并仅用fresh witness替换目标旧见证。

修正后`history_subtask_command_vnext.go`SHA256为`2093c2b7793fabf2f2ecb06243d4a06328ba50f36d9ebe369c89877c75a83c23`。失败到修正间没有其他production改动。

## 回归门禁与真实边界

`gates.tsv`登记：
- fast：`TestHistorySubtaskActualIndependentFence`。直接对每actual动作先验合法core delta／full-inspector seal，再调用独立fence；delete合法delta必须过，未授权字段污染仍拒。
- symptom：`TestHistorySubtaskActualIndependentOracle`。使用独立字段常量构造after，走完整`auditActualSubtaskHistoryVNext`，断言delete身份、位置、取消witness与父项全冻结。它是受影响纯helper的真实调用路径，不冒充UI、持久化或用户生产路径。
- aggregate：`scripts/check`。现有根模块`go test -p=1 -race -buildvcs=false ./...`强制运行taskapp两门禁；`TestFailureGateRegistry`验证函数真实存在及该聚合调用没有失联。

候选最终实测：新子任务focused exit0，2.965s；全taskapp exit0，34.993s；全taskapp race exit0，218.604s；vet exit0。另worker专属fence复跑exit0；只读实施审查最终focused exit0，3.098s。首race运行时仍存在另一个非法note墓碑测试fixture而红，修正fixture后完整race重跑绿，未把首红冒称通过。实际门禁与覆盖进一步记录于`../../tasks/audit-history-subtasks/README.md`。

新增登记后已通过仓库`scripts/environment`准备相同官方Go／已核依赖，重跑两真实fence/oracle门禁exit0，0.095s；`TestFailureGateRegistry`实际执行exit0，0.091s。没有仅靠文本登记声称接入。

本批已在e35共同基线整合双方测试，全scripts/check退出0（根/UI race541.422秒、taskapp244.296秒），六目标及隔离CLI通过；独立未读作者测试的11组focused/race通过，详见任务INDEPENDENT-ACCEPTANCE.md。修复 commit 为 ed200ac5d4e269fb23c884b3f19d817f928e8d87；GitHub MyGo run 42191600703 / attempt 1、job 127804716540 于2026-11-06 09:19:02 UTC成功。实际 artifact 13491585994 的6193字节 ZIP SHA256为8ab13654b385ae73a434824fc6bee55b8045e831c1aa97b0eed24f5a0f9a5381，与metadata及上传日志一致；完整日志含四模块race与六目标编译及隔离CLI。非verbose日志仅包级成功，不声称逐条打印测试名。来源及限制见任务目录HOSTED-CI-42191600703.json。本案不涉及安装、签名、启动或发行产物，故不虚设release gate；若后续切writer/UI，另须对应真实symptom、重开与故障门禁。

## 回退与教训

没有发行或切换有问题的helper，不需迁移／回滚用户资料。必要时撤销新helper与封闭注册即可，不能删除cache或用户数据冒充回退。后续为固定canonical期望新增字段时，先查schema中的精确wire类型，再以真正bridge output及独立字段oracle对照；不能因Go身份类型命名相似而推断所有ID都编码成rawValue对象。
