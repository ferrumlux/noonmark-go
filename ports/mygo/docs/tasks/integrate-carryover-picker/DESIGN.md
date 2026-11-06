# 日期组件与 Selected 依赖整合设计

task-id: integrate-carryover-picker

## 目标与前提
正式基线为 14a956c。此批仅整合已独立验证的纯日期选择组件，以及修复普通 Button 原生 Selected 语义的固定 MyGo patch；不启用 Day 业务入口，不引入尚在审查的异步草稿恢复及 pending variant。原 MyGo、上游 Swift、候选原冻结文件与历史红例保持不改。

组件源自最终三文件冻结，原生 Selected 症状在旧四 patch 依赖为红、六 patch 为绿；独立交互、像素与动画门禁完整保留。原作者 KnownAccessibilityMapping 是旧缺陷诊断，正式整合时明确更名并反转到修复后的 Selected=true、Checked=false 合同，旧文件及原红日志保留作证。不能只隐藏该测试，也不能把 RoleButton 改成 ListItem。

## 实施边界
- 在依赖作者与独立审查最终冻结及六平台体积测量完成后，按每个文件 SHA256 导入 0005/0006、manifest、NOTICE、README、专项脚本、fast/symptom 测试与设计案例。
- gates.tsv 只追加 GO-MYGO-SELECTED-001 一行，保留正式基线其他案例。
- 导入纯组件的源文件、作者测试与设计，再导入原字节独立 reviewer 测试。记录唯一有意修改的作者诊断测试的 before/after。
- 默认 scripts/check 已调用 check-mygo-native，后者 Test(Noonmark|...) 正则执行两项新 framework 回归，root ./... 执行 app fast/symptom；不额外重复 prepare 或绕过已有门禁。
- 保留 public API、Checked/Mixed 与 legacy 选择角色合同。三 OS adapter 改动不能被称为系统读屏实机验收。

## 验证和完成
正式整合后运行全量 scripts/check、专项 Selected 检查、六目标构建与实际隔离 Linux CLI，检查所有新文件已追踪。先提交实现，再以实际 fix commit 回填故障案例并跑 registry。生成及校验完整本地 Git bundle。因 GitHub 旧写入待批，禁止远端写入、重新请求同一操作或触发新 CI。

新的依赖归档与完整源码需要在最终被接受的正式源码上重新执行候选打包及离线源码重建；不能借用 a91 的历史成功作为本批证据，不发布版本。业务接线后也需重新对应源码验证。

## 风险与回滚
任何受害路径回归失败都阻止提交或入口启用；不删减断言。不改变已有资料格式，不访问生产或真实用户数据。可以回滚本批 UI 及依赖提交，但不能回滚此前已写 deferred 资料的读取能力。真实 Swift、系统窗口／IME／读屏与完整原版 undo/toast 仍未完成。
