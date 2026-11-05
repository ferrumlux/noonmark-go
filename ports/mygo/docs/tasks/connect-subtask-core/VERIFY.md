# 子任务领域与事务验收

本批仅 core/Service，不把后续界面候选算作已接通。所有输入均由领域命令合成，外置 build root，无生产资料。

## 永久回归

- 核心计划项：原地编辑、删除紧凑position、同值no-op原字节、错current definition、已有排期/放弃链、非法身份和时钟原子拒绝。
- 核心实际项：只改child时钟、显式父完成、未来编辑/今天完成边界、已完成child可改、删除保留取消墓碑和slot、新增身份/lineage/取消身份唯一。
- Service：计划→实际、回池→新计划→未来重排、陈旧owner、未来到今天、结算、重开精确保留。子任务全完成不自动完成父项。
- 标题canonical no-op、真实CGJ、非法UTF8、外部并发写入、历史磁盘损坏；确认值必须为实际保存字节。
- 标题以及其余九种写入命令全部覆盖 before-reserve、after-reserve、before-commit、after-persist。未确认不得返回新ID/标题；内存保持旧事实；半事务拒绝重开，完整但未确认事务重开可读确切新事实，不自动重试。
- 同Service四并发标题写入，每次确认对应自身请求、revision递增、反向墙钟仍保持数据单调，重开字节一致。
- 专用审计独立重放，拒绝伪造before/after、父内容、lineage、priority、时钟、分类catalog及无关supplement变化。
- 独立审查原错误反例和两个undo-alias探针永久保留。见 [GO-RUNTIME-012](../../engineering/failure-cases/GO-RUNTIME-012.md)。Swift最小测试仅作为外置后续oracle材料，未运行。

## 执行记录

初轮 core race1.284s、Service现有全套race17.439s通过。独立事务审查在单独副本执行新增Service回归4.460s通过；Add非pending父项错误类别原例为红。修正后正式候选core race1.300s，TestSubtask|TestReview Service race7.244s通过。后续完整验收已执行，结果见下节；本段focused通过不替代完整验收。

上游源码测试库存146条是宽匹配候选，不是146条已执行Swift测试。没有Swift工具链、原生显示服务器或macOS/Windows实机，不宣称平台完全对等。

独立复核：原副本原例仍exit1；修正副本原例、日期矩阵与locked×past×completed组合race1.029s、Service全部TestSubtask/TestReview race6.450s通过。额外九命令×四故障阶段和并发确认/反向墙钟测试race5.495s通过。全部原拒绝操作验证graph不变。

## 完整整合

实现提交 [cfa4f575990e5c93a05d65b00ad523b6e837bf49](https://github.com/ferrumlux/noonmark-go/commit/cfa4f575990e5c93a05d65b00ad523b6e837bf49)。候选与正式副本的 scripts/check、六目标cross-build、Linux development CLI均全部exit0。正式执行2026-11-05 19:53–19:56 UTC，main race54.731s、taskapp24.366s、core1.277s、native ui2.715s/root1.046s；51项依赖准备门禁全部通过。day/pool各2400×1536离屏输出，不是原生OS验收。

13个冻结候选文件哈希全部匹配，另仅调整运行README状态说明。完整日志外置保存为 `noonmark-subtask-integrated-{check,cross,entry}.log`；候选buildroot保留各阶段原始日志和源码SHA256清单。新head的GitHub CI须另外实际运行核对，本段不借用Unicode批次的绿灯。
