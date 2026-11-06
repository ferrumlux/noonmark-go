# 候选本地验证回执

状态：已修复；正式bff7414精确hosted run 42240698683通过。以下本地回执保留当时的验证范围；后续状态见文末收口。

- 基线：9802d246dc3960817f722ab844e1fff8a9ec06df。生产范围仅runtime_day.go/runtime_subtask.go，经过独立diff／SHA审查；只有一次liveDayView调用内私有graph传参，不跨view重建缓存。
- 原红：默认整根600.029s超时；实际Tester 2/34行每view Snapshot7/71次。原hosted42222499113为600.036s累计超时。
- 确定性绿：无child及有child展开list＋右detail，2/34行每view均1次Snapshot。全部5个新增Snapshot测试与精确freeze chain／旧Service／旧Day／新overlay负例已独立race复核。39顶层focused回归run/pass一致、0skip。
- 年度profile：原216.08s、候选185.69s；child编译0.699s／12.111s说明cache热度不同，不称净wall A/B。累计分配22,256→13,882MB；titleReadView链8,721→228MB；childRows整图复制链消失。完整年度交互、历史、磁盘与重启断言未删。
- 原scripts/check：2026-11-06T14:06:22Z开始，退出0；root577.040s非cached、taskapp277.291s，四模块race/vet、Unicode16完整Normalization/Grapheme/NFKC_CF及27路径负例、依赖51项、native与host popup均过。未传timeout或filter，没有跳过／削减测试，没有清理共享cache。
- 原scripts/cross-build：linux/amd64、linux/arm64、windows/amd64、windows/arm64、darwin/amd64、darwin/arm64，六项编译，退出0。
- 原scripts/test-entrypoints：隔离Linux development真实day/pool各2400×1536渲染，identity/create/reopen/reuse guards通过，退出0；全部完成于14:26:54Z。未验非Linux实机、桌面窗口或IME。
- 保持GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB，Go1.27.3，外置BUILD/HOME，原脚本race/default timeout不变，production/iCloud/Keychain/live AI均未访问。

## 保留证据

外置诊断证据保留root-tests.jsonl、timing-summary.json、原/候选年度CPU与mem profile、逐view红绿、expanded与39项focused日志、独立审查、candidate-check.log和各阶段exit、六二进制checksum、source-manifest.json与完整验证回执。证据是本次合成development/e2e结果，不是用户production数据。

## 当时剩余验收（已由下方精确hosted回执补齐）

父任务整合候选、核精确commit及远端备份，再跑新head真实hosted CI。577.040s只有约23s余量，不能提前宣称任何环境都已解决600s问题。若hosted失败，保留失败终态重新定位，禁止调timeout或删门禁。

## 精确 hosted CI 收口（2026-11-06）

[run 42240698683](https://github.com/ferrumlux/noonmark-go/actions/runs/42240698683) attempt 1 于15:33:28Z成功完成。正式head为bff7414d71c10f31f45c679393e035aa6386b989；本地候选0b33571a1d3892201d9c457051db25c6a45eaabb与它的tree均为04482b01e72640b808d7d49c65b7acf728fadfc3，但两者不是同一commit。此证据不改绑到后续文档／测试提交。

原默认600s根包race在492.032s完成，余107.968s；四模块race/vet、Unicode16、51项依赖合同、默认native及host popup、六目标编译、Linux合成CLI全部通过。TestLayoutInputs、TestNoonmarkSelectedLayoutCache及其五角色子测试明确PASS，另两项Selected状态／顺序测试也PASS。根应用测试为包级非verbose输出，不声称逐项名称都单独记录。

公开精简回执：docs/tasks/diagnose-root-test-timeout/HOSTED-CI-42240698683.json，记录来源、job／artifact身份、摘要和计数。6435字节artifact的SHA256为8eb73ce4212ca68ec6aaa514499e5a9e9dfef48ca3055ba877aeded25f50e1c8，与GitHub及上传日志一致；六个安全成员CRC通过，阶段文本与完整job日志对应。

仅关闭GO-PERF-DAY-001的本次精确head超时缺口及GO-MYGO-LAYOUT-001的对应hosted证据待项，不改变其它案例。单次600s内成功不保证未来runner性能；完整framework allocation／TestDiffFrameAllocs、ICU76、真实桌面／IME、六平台runtime、新Swift、签名／release和完整产品对等均未获此证据验收。产品源码、fixture、默认补丁、冻结清单和许可证不变。
