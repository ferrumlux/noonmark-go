# GO-NATIVE-WINDOW-FREEZE-001：窗口探针遗漏历史冻结来源重绑定

状态：本地初始整合门禁真实失败；窄范围修复已通过全部 Frozen 定向 race（1.093秒）及 registry（0.078秒）；独立审查、完整回归仍待完成。关联 #9，不能据局部测试或工作流代码称六平台窗口通过。

## 原始症状

本地 7e4487d81bb95460d9c24cb68d0740b97712ddac 加入显式 development 窗口探针，修改 main.go，但没有为历史审计的冻结生产清单提供新的精确来源 overlay。2026-11-06 的 scripts/check 根应用550.459秒通过，internal/taskapp 在251.162秒结束时 TestHistoryEditFrozenProduction 拒绝 ports/mygo/main.go。整批退出1、864秒；六目标 cross-build 与 CLI 阶段没有执行。该失败没有推到 main，也没有用 skip 或删除冻结清单绕过。

## 最小修复与永久门禁

仅在测试合同添加一个固定 old/new SHA256、精确 main.go 路径的 overlay，串在原四层之后。原 manifest、四个既有 overlay、全路径 hash 循环及历史 command suffix 校验保持不变。生产 Go、窗口驱动和 CI 本次修正均不改。

- TestNativeWindowFrozenOverlayRejectsUnapprovedChanges：拒绝未知 hash、路径／别名、缺失／多余／重复行、坏格式和错误／已应用基线，证明只改变一个允许路径且不修改输入 map。
- TestDayReadFrozenConsumerChain：原链仍按顺序执行，新层恰在最后；不得移除旧层。
- TestHistoryEditFrozenProduction：原始实际字节症状回归，仍逐文件比对并核旧历史 command suffix。
- scripts/check 保留默认 race、vet、四模块及全部附加门禁，不改600秒测试上限。

失败日志保留在隔离构建证据中；后续只对精确最终版本记录真实绿例。恢复原 overlay/test 接线会重新触发同一 main.go 拒绝，不能用旧冻结值声称新入口已通过。教训：受审新增开发入口仍改变冻结来源，集成必须连同严格来源绑定及原症状门禁一起核验。
