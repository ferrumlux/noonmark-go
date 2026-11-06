# GO-RUNTIME-CARRYOVER-UI-001：异步延期后的原生草稿生命周期

状态：已修复（仅本轮隔离 ordinary Day 的 headless 合同与原生 Tester 路径）；修复提交 `961e688dcf0077c7976a61e6f5e1279ef6254960`，正式整合的完整 check、六目标编译和隔离 Linux CLI 均通过。真实系统 IME／六平台桌面／Swift／正式发行仍未验收。

首次发现：2026-11-06（新加坡）；合成复现的可见症状为另一窗口在命令等待期间输入子任务，延期后来源历史化，原生输入字段消失；另有已清空的失败新增被错误阻断、在途日期仍可改却提交旧目标。

引入提交：未提交隔离候选，非既有正式commit。Git author／committer不适用；问题引入于本轮未提交的 Day 接线实现，独立测试提供真实 Tester 原红。没有证据将问题归因到上游Swift或生产发行。

根因：只在准入前检查composer；publication仅Snapshot却未撤销native输入lease；恢复只覆盖Add失败；条件sidebar控件增加还曾改变原生祖先scope。pending选择器未绑定不可变命令状态。

修复：UI线程在旧node尚在时真实Finish；Finish专用proof与Snapshot区分，失败保留原对象native字段；独立carryoverRecovery不伪造Add失败；永久keyed恢复anchor稳定host scope；跨页面原任务入口只在恢复状态出现；原生明确空白解除恢复但不清pending/ack。日期组件可选pending入口禁所有目标修改，Cancel不倒写盘。

门禁：fast为同源readiness、跨窗失败原生清空、pending目标不变；symptom为无Frame直接marked输入、失败Finish仍保留node、旧client租约撤销、跨页多来源恢复及显式清空Close。全部登记入scripts/check。独立原红完整保留，未修改其断言。

证据目录为本轮显式外置BUILD/evidence，含binding-lifecycle-marked、binding-native-host-scope、binding-pending-recovery-multi。源码token／headless不代表真实系统IME与Swift oracle。正式及平台发行尚未运行，不声称release通过；回滚仅去掉入口，不删除已持久化事实。

教训：异步事务的UI正确性必须覆盖准入前、writer在途、confirmed publication和原native退场；空Snapshot不能代替可靠Finish。恢复“Go对象还在”不能代替真实native字段可发现、可复制、可明确清空。

R4补充根因：publication以前的兄弟窗口Close、原图非deferred但先Complete/Return导致owner退场、四阶段失败后的晚输入、以及重复publication撤销恢复新lease，均有真实独立原红。修订统一使用渲染predicate的owner退场边界、guarded导航边界和stable恢复owner，而非domain状态特例。共享source引用只在实际准入后由admission hook注册，completion保全之后释放；同步callback亦验证。永久恢复anchor还限制为footer28高，不覆盖整个sidebar；旧全量捕获的18项导航点击红使用原断言回归，不绕过hit测试。

R5自动退场补充：午夜跟日原来只判断标题，Finish失败后仍清selection；排队的空标题清理又会撤销稳定恢复字段重新取得的native lease。统一自动离页许可要求受保护owner的真实Finish证明；非离页标题清理保留普通composer屏障，仅跳过稳定恢复owner。原午夜成功/失败双分支、排队清理原红和普通Continue场景均保留。

R6根因补充：自动标题处置原复用用户navigation target，会在publication晚到时覆盖用户已点击的页面切换。改为独立request与精确同步TryRetireEditors，仅处理捕获registration/session identity；用户intent优先，无自动重试/保存/导航。保持所有原红，病例状态仍为处理中。

R7合同补充：合法Changed-hook重入新导航（非已证实OS路径）使request失效，但coordinator精确退休已经成功；旧app后置fence忽略成功会留下悬空注册。成功事实收尾必须仅按原pointer/reg/identity协调，不因展示失效丢弃，也不能清新owner或新导航。原合同断言及new-pointer/same-pointer-new-registration对照保留。

闭环：修复提交的 Git author／committer 为 Codex <codex@openai.com>；这仅是 Git 署名，不据此推断其他人的实际操作。全部十份独立原红逐字保留，补充 coordinator native／Changed 重入测试也纳入默认聚合。正式 root race 261.873s、taskapp 55.885s，六目标与 CLI 退出均为 0；源代码与执行范围见 `../../tasks/connect-carryover-ui/VERIFY.md`。此前各代局部修订与原红记录保持，R7 整体通过后才关闭此案例。
