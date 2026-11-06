# diagnose-root-test-timeout

状态：已修复；正式bff7414精确hosted run 42240698683通过。Task-Id：diagnose-root-test-timeout。详见VERIFY.md及下方收口。

## 需求／现状

正式9802d246dc3960817f722ab844e1fff8a9ec06df的hosted run 42222499113在根包默认600s超时。唯一未修改整根计时再次600.029s超时，294项已过；Swift比较27.64s已过，超时移到后继离线oracle合同，不能把累计预算耗尽描述成Swift运行失败。

年度真实Tester＋Service路径220.73s；原单项race profile216.08s，累计分配22,256.07MB，Clone56.59%，childRows链8,271.16MB。每行测量／绘制各复制整图，即使没有子任务。GC后台mark占27.05%CPU，未符号化外部35.15%与racecall8.76%仍保留未知归因。

## 根本候选与边界

仅runtime_day.go/runtime_subtask.go：每次liveDayView调用取新私有Snapshot，内部测量／row／detail／子任务section显式传graph与today。MyGo下一次view build（含同一Frame重建）必须重新读取，没有跨view缓存。editor合并每次重算，不缓存dirty／retained／failed结果。交互与原生回调继续fresh读取，绘制图不授予写入权限；Snapshot保持防御性深拷贝。ProjectDay、core、store、数据校验、race、资源及timeout均未改。

原Frozen manifest、Service/Day overlay、旧validator与历史loop/suffix逐字不变。新增严格性能overlay仅精确两path/old/new，独立审查和缺项／重复／未知hash／错误旧基线等负例通过，不允许宽泛豁免。

## 实测与验证

实际Tester红绿：2/34行每view Snapshot7/71次降为1/1次；有children且展开list＋右detail也为1/1次。五项新增测试包括连续真实磁盘revision、新view刷新、旧私有图不变、修改副本不污染事实、dirty／失败／删除retained时序、旧native提交排队后新revision拒写并保留草稿。39项focused及独立race复核通过。

候选年度全部原断言通过185.69s；原profile216.08s，但child编译12.111s／0.699s，cache热度不同，不称纯净wall A/B。累计分配22,256→13,882MB，titleReadView链8,721→228MB，childRows复制链消失。

原scripts/check完整exit0，首次完整root577.040s非cached，默认600s且新增约24s门禁全计入；taskapp277.291s。四模块race/vet、Unicode16、依赖51、native通过。原六目标cross-build与Linux development真实day/pool CLI均exit0。没有删fixture、动作、截图/hit/access或历史/disk/reopen断言。

## 本地验证时的剩余风险与处置

本地root仅约23s余量，不能保证任意runner性能。父任务整合、备份后须核新精确head hosted CI；非Linux目前仅编译，不称实机或IME验收。失败须保留终态重新定位，禁止调timeout或削门禁。回滚只还原本候选两文件读取传递及新增登记，不迁移／删除／访问用户production资料，不降低既有受害路径门禁。

## 精确 hosted CI 收口（2026-11-06）

[run 42240698683](https://github.com/ferrumlux/noonmark-go/actions/runs/42240698683) attempt 1 于15:33:28Z成功完成。正式head为bff7414d71c10f31f45c679393e035aa6386b989；本地候选0b33571a1d3892201d9c457051db25c6a45eaabb与它的tree均为04482b01e72640b808d7d49c65b7acf728fadfc3，但两者不是同一commit。此证据不改绑到后续文档／测试提交。

原默认600s根包race在492.032s完成，余107.968s；四模块race/vet、Unicode16、51项依赖合同、默认native及host popup、六目标编译、Linux合成CLI全部通过。TestLayoutInputs、TestNoonmarkSelectedLayoutCache及其五角色子测试明确PASS，另两项Selected状态／顺序测试也PASS。根应用测试为包级非verbose输出，不声称逐项名称都单独记录。

公开精简回执：docs/tasks/diagnose-root-test-timeout/HOSTED-CI-42240698683.json，记录来源、job／artifact身份、摘要和计数。6435字节artifact的SHA256为8eb73ce4212ca68ec6aaa514499e5a9e9dfef48ca3055ba877aeded25f50e1c8，与GitHub及上传日志一致；六个安全成员CRC通过，阶段文本与完整job日志对应。

仅关闭GO-PERF-DAY-001的本次精确head超时缺口及GO-MYGO-LAYOUT-001的对应hosted证据待项，不改变其它案例。单次600s内成功不保证未来runner性能；完整framework allocation／TestDiffFrameAllocs、ICU76、真实桌面／IME、六平台runtime、新Swift、签名／release和完整产品对等均未获此证据验收。产品源码、fixture、默认补丁、冻结清单和许可证不变。
