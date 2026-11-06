# 修复按钮 Selected 语义

## 需求与现状
原版 DatePickerDayButton 是 Button + accessibilityAddTraits(.isSelected)。组件调用 ButtonBase.Selected(true).Background(Transparent) 已保留正确像素，但官方 MyGo 977ea57 的实际 Access 树为 RoleButton、Checked=true、Selected=false。不能改角色、改原红断言、只翻译 Tester 或以中文 Value 冒充状态。

## 设计
Selected 独立存储 unset/false/true，与 Checked/Mixed 不互相覆盖；保持现有 accent styling 和 Background 后置覆盖。AccessSelectionKnown 追加在原 bit enum 末端，无既有位值变化。显式 false 优先于旧的 role-based Checked selection。旧列表/树/行/tab 未设置 Checked 时保留旧 wire alias，真实 checkbox/radio/toggle 的 Checked 不变。树 hover/arrow 和 tab roving focus 读取独立 selection，保留原行为。

macOS 更新 AX selected（含清除），tab AX value 保留选择语义；Windows 独立 SelectionItem 与 Invoke，可同时有 Toggle 通知；Linux ATK selected/selectable 与 checked 分开。没有生产、iCloud、Keychain、live AI 或远端写入。

## 复现与供应链
新补丁005/006由官方归档加原四patch构建的准确前态生成，manifest固定每文件前后SHA256、补丁SHA256、Go archive sum与MIT licence。现有准备器继续离线fail-closed并产生只读外置workspace，绝不覆盖模块缓存。release依赖收集读取prepared module，SOURCE.json包含完整manifest；源码包收集所有已追踪patch及NOTICE，无四patch硬编码。正式整合者须将新文件追踪并重做对应正式head打包，不把未提交候选误称可发布产物。

## 门禁与限制
快速框架 TestNoonmarkSelected*；永久实际组件 TestCarryoverPickerSelectedAccessibilityContract。原独立 TestReviewCarryoverNativeAccessibilitySemantics 与 TestReviewCarryoverHoverAndSelectedPixels 不改一字，另做red→green证据。作者 KnownAccessibilityMapping 旧测试专门断言缺陷存在，整合者须经作者许可更新其预期，不能将其失败误归新回归。

真实OS读屏、系统事件顺序及Swift像素仍待实机；六目标仅编译。既有 unrelated TestDiffFrameAllocs 基线失败，不声称整个框架全绿。

## 实际执行次序（不得当作实施前设计）
本专项先阅读 AGENTS、既有patch README/manifest、准备脚本、既有视觉设计及组件设计，再在隔离framework实施并跑初轮门禁，随后才将本专项设计落盘。故未满足“专项设计先落盘后实施”的流程要求，不倒填日期或声称事前审读。现已停止追加源码修改，提交父线程审读。首次扩大回归出现旧list raw AccessChecked不再有值的失败，后续保留仅选择角色的兼容alias后race回归转绿；原失败日志保留。流程偏差与实现结果分别陈述。

## 审读后的追加测试计划
父线程审读后允许继续；下一步仅扩充自动测试：对Selected true/false与Checked true/false、Mixed组合遍历两种调用次序，验证实际AccessTree原始状态及重绘清除，不通过修改症状断言掩盖问题。已有真→假→移除Selected重绘与Button click/Space门禁继续保留。

## Native adapter 精确变更范围
- macOS：valueOf 对Tab用独立Selected作为AX value；更新周期在Tab独立选择变化时触发value更新。setAccessibilitySelected: 从仅chooses(row)扩为显式选择节点，也包含前帧显式节点以清除失效状态；触发条件为fresh、chooses转换、selected布尔值变化或known标记变化。AXSelectedRows集合读取Selected()。保持RoleButton和现有press action，没有引入新ObjC ABI或新用户动作。
- Windows：显式选择节点暴露SelectionItem，原Invoke判定保持不变。get_IsSelected、Select/AddToSelection短路、容器选中项读取Selected()。独立selected变化发UIA IsSelected属性变化及原有选择事件规则；Toggle和SelectionItem通知由互斥switch改为独立if，让二者共存。RangeValue仍按原值变更发事件。没有更改RemoveFromSelection既有不支持策略。
- Linux：ATK state set在显式节点加selectable/selected，原checked状态按角色保留；列表选中集合及is_child_selected读Selected()。已有state diff通知机制不变，因state set改变自然报告选择变化。原角色、按键和动作分派不变。
- 以上仅源码和跨目标编译验证；没有执行Cocoa/UIA/ATK真实辅助技术setter/event验收。

## 独立审查后的清除边界修正计划
原首轮freeze撤回：独立审查指出Windows仅新帧有SelectionItem时才发选择变化，Button移除Selected后会漏true→false；macOS仅旧legacy选择角色转为Button也未包含旧chooses条件。先记录方案：Windows提取纯selectionItem支持判定，notify在前/后任一帧支持时检查布尔差异；macOS setter覆盖chooses(previous)。追加Windows纯支持/转换门禁并跨目标test编译（此host不执行Windows测试），保持实际OS事件未验证限制。

## Registry 接入校正计划
自查发现 failure registry 只接受应用源码可发现的测试名及单一scripts/check入口，不接受依赖包测试名。追加应用层通用Button快速门禁，与组件症状门禁一起登记；依赖专项仍由check-mygo-native和check-selected-access覆盖。不扩大registry解析器。

## macOS 选择变化通知校正计划
独立审查指出setAccessibilitySelected:不应被假定会自动发可访问事件。下一步在已有apply返回路径中，对非fresh、选中布尔值改变且非列表选择row的节点标记valueChanged，由既有UpdateAccessibility统一发AXValueChanged；列表row继续使用原AXSelectedRowsChanged。这样Button选择变更和清除有明确通知路径，不仅setter赋值。真实VoiceOver接受方式仍待实机验证。
