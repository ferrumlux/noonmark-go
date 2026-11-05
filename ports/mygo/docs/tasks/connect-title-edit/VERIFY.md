# 标题命令批次验证

2026-11-05。仅合成 development/e2e 资料，Go 1.27.3 linux/amd64。没有新增可见 UI 编辑器。

- 最终 scripts/check 通过：主包 MyGo 实际视图/Day/Calendar/任务池交互及 race 38.187s，taskapp race 8.898s；taskview 与三个领域模块的所有包 race、vet、gofmt 均通过。GO-RUNTIME-006 两层门禁已纳入 registry，额外独立审计测试也已落入 taskapp。
- 独立审查未发现可复现历史绕过；新增 six-field 攻击与损坏前缀 no-op 回归。原 CaptureEmptyHistory 源码未修改，旧严格拒绝测试仍通过。
- 六目标交叉编译通过：linux/windows/darwin 的 amd64/arm64。只证明可构建，不是各平台窗口、字体、IME、文件系统耐久性或设备运行验收。
- scripts/test-entrypoints 通过：固定 development 身份、新会话/重开/reuse 守卫，Day 与任务池相同 MyGo 代码离屏渲染 2400×1536。
- 首次 cross-build 在四目标后因临时盘满失败，已保留源码/会话/证据，迁出此前阶段的可再生缓存后重跑完整 check、六目标编译和入口测试，全部通过。没有跳过失败目标，也没有修改编译代码来绕过环境限制。
- 证据日志留在指定外置 build root 的 evidence/title-{check,cross,entrypoints}-final.log，不上传本地运行数据或二进制。报告中的通过结果来自最终成功命令，不用先前失败流程的后续 echo 冒充成功。

未验收：Swift/Foundation oracle、原生三平台/arm64、真实输入法及退出协调、断电耐久性、生产储存和 v7 Swift 互通。700ms autosave 和标题 UI 属下一批；不能将新核心/Service API 描述为用户已经可编辑详情标题。
