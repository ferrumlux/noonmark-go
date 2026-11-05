## 当前状态（2026-11-06，新加坡）

日待办 pending 且 date>=today 的原生多行标题编辑已接入隔离 development 应用：700ms自动保存、真实Handle快照／最终化、统一写入lane、可取消Close／Quit及失败留稿。完整实现、最终证据与安全差异见 [NATIVE-UI.md](NATIVE-UI.md)。下面报告保留为当时的前置设计／验证记录，其中“只读／尚未开放”等描述只适用于该历史批次，不代表当前运行状态。

Linux headless与六目标编译不是实机IME或Swift动态oracle；这些限制及原始未修复依赖红色探针继续保留。

# 标题 autosave 前置批次验证

2026-11-05。基线 986cd8d，所有状态与资料均为合成 development/e2e。

## 已通过

- 纯 inputdraft：36 个顶层测试，含 8 个官方 gate 行为改编；有界遍历 248,832 条长度5事件序列、271,452 次转换。独立候选 statement coverage 100%，不代表所有状态或 Swift oracle。
- 独立源码审阅核对 sticky flush、并行请求门禁、新编辑不会被旧确认清除、NFC owner 比较、wrap、unsigned max 与失败重试顺序，未发现可复现差异。
- Service 新 trace 路由：当前/未来、多行、重开、旧历史 trace 与同链新 pending、同日 completed 与同链未来 pending、canonical no-op、stale/blocked/closed、四阶段故障及自然日并发全部通过。旧链级入口与 strict CaptureEmptyHistory 不变。
- 最终 scripts/check：主包及 MyGo 实际视图回归 race 34.939s，inputdraft 1.261s，taskapp 10.511s；其余 taskview、core、classification、recurrence、codec 包及全套 vet/gofmt 通过。
- 六目标交叉编译全部通过：macOS/Windows/Linux 的 amd64/arm64；CLI development 身份/新建/重开/reuse 守卫与 Day/池离屏渲染通过。

## 已复现但未修复的 UI 接线阻断

`scripts/probe-title-native-input` 在锁定 MyGo 的精确外置源码副本注入原生 client 测试。两个简单关闭样例通过，`marked-selection-before-frame` 原始症状失败：captured 为空，expected 为合成的 marked text。退出码明确为1，不写成通过；详见 GO-RUNTIME-007。脚本不能直接 overlay module cache，因此复制锁定源码到临时 build 子目录，不改模块缓存或原框架仓库。

最小 focus-finalization 候选在独立 overlay 已令相同三个探针转绿，但尚未作为正式依赖启用，也未验收完整退出/多窗/遮挡窗口流程。本批不宣称修复标题输入 UI。Service 与纯 gate 已就绪不代表能安全省略原生最终化步骤。

## 证据与边界

聚合日志在外置 build root 的 evidence/autosave-{check,cross,entrypoints}-final.log；原始红色探针为 title-native-probe-red.log。源码仓库不含会话、工具链、二进制或个人资料。

仍未完成：可见标题输入、700ms实际UI持久化、完整Markdown标题显示、原生平台/IME/退出实机、Swift oracle、断电耐久性及生产迁移。当前详情继续只读。新前置 API 不改变会话格式，也不放宽 runtime scope。
