# 接通标题自动保存

task-id: connect-title-autosave。级别 P。2026-11-05。

## 需求、现状与设计

目标是原版详情标题的多行输入、700ms debounce、IME 隔离、空白失焦恢复，以及切换和退出时持久化确认。基线 986cd8d 已有两个链级标题命令和专用历史审计，但界面仍只读，没有可冒称已完成的 autosave。

先补两个明确前置条件：

1. trace 级 Service 保存入口。同一锁内核验所选 trace 仍 pending 且 date>=today，随后进入既有标题事务，禁止旧历史详情回调借同链新 pending 修改标题。UI 的 Snapshot 检查不能替代这个原子边界。核心仍按 current definition 定义 title 版本。
2. 独立纯状态 IME gate。按上游 owner/generation/revision/sequence/request 模型阻止过期保存、composition 写入和并行确认；保留 700ms、显式 flush、失败退避和 discard 行为。它不拥有 timer、窗口、网络或存储。

之后才接入 MyGo 实际文本事件、可取消的 Close/BeforeQuit、跨 owner draft、异步完成回派与真实 Tester 回归。当前不引入原版没有的 Save/Cancel 按钮；不把模拟 composition 测试说成真实原生输入法验收。

## 风险与验证

继续仅用合成 development/e2e 资料，原 snapshot/history/scope 校验不放宽。写入不确定时保留草稿并拒绝退出，不能自动删除或修复 session。自然日与完成状态竞争必须由 Service 同锁决定，UI 保存失败不能改历史。

本批每个部分需 focused race/vet、独立审查，再跑 scripts/check、六目标编译及 CLI 入口回归。未满足真实输入、失焦、切换、Close/BeforeQuit、失败及重开完整路径前，仍保留只读标题 UI。无生产迁移；新 API 与未启用编辑器可回滚源码，测试资料不自动清理。
