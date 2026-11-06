# GO-PERF-DAY-001：日视图逐行整图复制导致累计测试预算耗尽

状态：已修复；正式bff7414精确hosted run 42240698683通过，限本案例默认600s门禁缺口。

- 首次明确发现：2026-11-06，hosted run 42222499113，commit 9802d246dc3960817f722ab844e1fff8a9ec06df根包600.036s累计超时。
- 影响：真实年度日视图与默认验证耗时过高；失败阻止后续模块／构建／CLI门禁完成，不是Swift运行失败，也不是Selected断言失败。
- 归因：9802d246加入Selected真实依赖门禁，Git author为ferrumlux；不能据此将更早存在的逐行复制归因于该提交。底层重复复制的精确引入commit、author／committer及实际操作者尚未知；现有checkpoint不能替代引入证明。
- 原红：同样默认600s整根600.029s失败，294顶层项已过，Swift comparison27.64s已过，超时发生在后继oracle。实际Tester计数2行每view7次Snapshot、34行71次，新增门禁判红。
- 定位：年度220.73s；profile累计分配22.26GB，其中Clone56.59%，childRows路径37.16%，GC后台mark27.05% CPU。UI每行测量与绘制先整图Snapshot，两者皆放大年度图复制。外部编译与其它真实验证也有耗时，不把全部600s归于一个调用。
- 排除／未证：不是Swift执行失败；未证GC参数是根因，不调参数。未符号化CPU保留未知。ProjectDay还有独立重复Clone，但本阶段不改；安全验证不得移除。
- 根本候选：仅绘制内部传本次liveDayView已取的私有图／today；下次view build重新读取，不跨frame或pass缓存。保留交互fresh包装及所有editor合并时点、domain/store校验与Snapshot防御性语义。
- 快速门禁：TestDaySnapshotOneReadPerViewBuild；不依赖wall阈值，准确验证每次view build读取次数。
- 症状与受害门禁：TestDaySnapshotRevisionsAndIsolation、TestDaySnapshotOverlayRecomputed、TestDaySnapshotQueuedComposerUsesNewFacts、TestCarryoverYearMiddleNativeService及原subtask／几何／native交互回归。聚合scripts/check，既有六构建与CLI发行前检查不变。
- 证据：隔离BUILD保留原红、focused绿、完整profile及SHA；任务入口docs/tasks/diagnose-root-test-timeout/README.md。本地聚合已通过；当时正式整合／新head真实CI未完前保持处理中，现已由文末精确回执补齐。
- 回滚：还原这两项UI读取传递及新增登记，不操作用户资料，不放松历史冻结／Service／Day overlay。无生产运行、iCloud、Keychain或live AI。
- 教训：计数并约束每次view build的事实读取，不能把每行深拷贝当廉价getter；性能修复以完整真实路径与不变安全语义对账，不提高timeout或削测试。

## 候选进展（历史记录）

精确两文件静态独审、五项Snapshot测试和严格冻结overlay独立race复核已通过。候选年度185.69s，旧profile216.08s；编译cache热度不同（12.11s／0.699s），不将wall差直接当净优化率。累计分配22.26GB降至13.88GB；childRows整图复制链消失，titleReadView链8.72GB降至0.23GB。本地完整聚合通过时，正式整合及新head hosted CI仍待；此为历史状态，现已由文末回执补齐。

## 本地完整回执（2026-11-06）

原样scripts/check退出0，首次完整候选root577.040s且非cached，默认600s，新门禁全部计入；taskapp277.291s；四模块race/vet、Unicode16全量一致性、27路径负例、51项依赖合同、native与host popup全部通过。原cross-build六目标编译退出0；原test-entrypoints Linux development真实day/pool各2400×1536渲染及identity/create/reopen/reuse防护退出0。六目标中非Linux仅编译验证，不冒称实机或IME验收。

本地root只余约23s预算，不能保证任意runner性能。当时新精确commit的hosted CI及正式整合仍待；现已由文末回执补齐。代码、fixture、旧门禁、资源、race、默认timeout及原入口均保持已审版本；文档在本地验证后更新，没有再改测试或生产代码。

## 精确 hosted CI 收口（2026-11-06）

[run 42240698683](https://github.com/ferrumlux/noonmark-go/actions/runs/42240698683) attempt 1 于15:33:28Z成功完成。正式head为bff7414d71c10f31f45c679393e035aa6386b989；本地候选0b33571a1d3892201d9c457051db25c6a45eaabb与它的tree均为04482b01e72640b808d7d49c65b7acf728fadfc3，但两者不是同一commit。此证据不改绑到后续文档／测试提交。

原默认600s根包race在492.032s完成，余107.968s；四模块race/vet、Unicode16、51项依赖合同、默认native及host popup、六目标编译、Linux合成CLI全部通过。TestLayoutInputs、TestNoonmarkSelectedLayoutCache及其五角色子测试明确PASS，另两项Selected状态／顺序测试也PASS。根应用测试为包级非verbose输出，不声称逐项名称都单独记录。

公开精简回执：docs/tasks/diagnose-root-test-timeout/HOSTED-CI-42240698683.json，记录来源、job／artifact身份、摘要和计数。6435字节artifact的SHA256为8eb73ce4212ca68ec6aaa514499e5a9e9dfef48ca3055ba877aeded25f50e1c8，与GitHub及上传日志一致；六个安全成员CRC通过，阶段文本与完整job日志对应。

仅关闭GO-PERF-DAY-001的本次精确head超时缺口及GO-MYGO-LAYOUT-001的对应hosted证据待项，不改变其它案例。单次600s内成功不保证未来runner性能；完整framework allocation／TestDiffFrameAllocs、ICU76、真实桌面／IME、六平台runtime、新Swift、签名／release和完整产品对等均未获此证据验收。产品源码、fixture、默认补丁、冻结清单和许可证不变。
