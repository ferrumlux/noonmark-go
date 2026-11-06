# GO-UI-DETAIL-001：Day 详情顶部偏离固定源码

状态：已修复（本地症状与相关回归）。精确修复候选ff4901ac0726061694f5793f93151090e2796d7a已完成整套检查；整合代码与候选一致，独立整合专项通过。正式同步和该提交的hosted CI另核，不是桌面或发行验收。首次确认：2026-11-06。

## 症状与归因

Day 任务详情显示裸状态、单独 ISO 日期行、多余的返回日统计按钮；没有真正关闭选中任务的 xmark。与固定 Swift StatusChip、DetailHeader、TaskDetail 不符。影响视觉密度、圆角和导航入口。

Git blame 可定位这些初始 UI 行至 d85021f6b47cf735623c698c57b127042d052979，author/committer 为 ferrumlux；保存前导航门随后在15d01017接入。Git identity 不等同实际操作者，实际修改者未知。不将字体差异当作几何错误的借口。

## 证据与根因

已实际查看固定上游 Day README 图与旧 Service 合成详情图。README 显示每日复盘，因此不作详情像素 oracle。根因是详情初始实现没有复用固定源码状态胶囊与标题层级配方，并增加独立返回行。

最初症状测试捕获缺失 xmark、多余返回行、错误日期。独立交互基线12叶场景均因缺少关闭入口失败。首次绿色日期断言发现 Tester.Texts 使用语义 Label 覆盖可见文本，改用明确的无障碍 Value 断言，不修改日期来迎合测试。

日期设计中的年份假设已排除：固定 AppDateTimeFormatter 明确 monthDay 为 M月d日，weekday 为 EEE。当前 UI 无英文切换，测试针对中文 zh_Hans_SG。

## 修复与门禁

采用按字体测量的 Capsule，固定点/padding/颜色；日期同排、窄栏整体换行；11/600/text3/.8 header；移除返回整行，以独立透明28目标 xmark 复用 titleTransition。正文14/600及所有保存和Service语义不动。cancelledDraft/未知状态不渲染，不 panic 或伪映射 pending。

fast gate：TestDetailHeaderSourceGeometry、TestDetailHeaderStatusColorsAndHiddenCancellation。
症状 gate：TestDetailHeaderOriginalSymptom、TestDetailHeaderMetadataPixelsAndWrap、TestDetailHeaderServiceVisualMatrix、独立 TestDetailClose*。
均由 scripts/check 的 race package suite 执行，登记于 gates.tsv。

## 验收边界及回滚

Service 合成12子任务、长标题、pending/completed/unfinished、100%、历史只读，在1200×768与960×720、1×/2×离屏渲染；另125 DIP窄栏换行。独立导航覆盖失败、重复、迟到、键盘、已送达 native composition 与25子任务长滚动。真实 OS IME、macOS相同状态截图、SF Symbols及continuous曲线仍未验；不声称平台对等或像素相同。

候选仅本地审查，不触发CI/发布；回滚整个候选提交恢复旧视觉入口，不需数据迁移。

固定 header + 正文 Scroll 是本端既有结构，未验证 Swift 祖先容器的整个布局/滚动契约。此案不将局部配方对齐扩大成完整详情布局验收。

首次完整0031检查：root/UI race591.475秒通过，taskapp283.257秒在TestHistoryEditFrozenProduction真实判红，尚未走到后续模块。没有修改原历史冻结manifest；正式整合追加两项已审UI的严格版本overlay，保留不相关文件和command suffix冻结。最终全检以ff4901ac的回执为准。

最终验收：ff4901完整scripts/check退出0，root/UI race591.150秒、taskapp274.060秒；其余模块含缓存命中，vet、Unicode、依赖51项与默认native全通过。六目标编译及隔离Linux持久化CLI通过。整合副本独立Header/Close/registry race45.301秒、冻结/Service overlay race1.027秒及root vet通过；整合后只有文档闭环差异，未另称重跑全集。证据哈希见同任务VALIDATION.json。

## 状态登记纠正

346d775同步后的完整diff复核发现：本轮整合脚本未按case ID限定状态字串替换，误将10个旧案例的status换成本UI候选文字。Git identity为ferrumlux，实际来源是本轮已记录的整合操作；测试入口、执行脚本、生产源码及数据没有因此改变。现按cf9a77f逐行恢复所有旧条目，仅保留已有Service CI闭环与本UI条目更新。新增TestDayDetailGateEvidenceScope真实读取登记文件，旧346d版本先判红；负例证明其它case不得冒用ff4901证据，正确UI条目可通过，已进入scripts/check与本案例fast gate。后续批次按case ID修改并完整核对diff，不能用无范围replace。
