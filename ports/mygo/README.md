# Noonmark Go + MyGo 移植

这是完整复刻的研发快照，不是功能齐全的发行版。20 场景原型与三批领域模块已集中在此目录；本轮已新增普通任务的真实 UI → 领域命令 → 隔离文件存储 → 重开回读链路。

## 当前可用

- 在 Day Todo 或任务池输入中文标题并添加，Enter 提交。
- 将任务池条目排到今天或明天。
- 完成/撤销今日任务，按真实事实更新导航计数和已完成/未来计划列表。
- 日切换、滚动长列表、详情选择、快速记录到任务池。
- 普通任务真实只读月历：5/6行、日期/月份键盘导航、完成数量热度、可展开日期详情及打开 Day Todo。
- 关闭并重开开发会话保留完整 TaskChain / Definition / Trace；自然日推进时以同事务结算任务和空分类历史。
- 写入失败保留草稿和原快照；并发冲突、损坏和不完整写入 fail-closed。

未接入的页面明确显示状态，不混入静态演示任务。原 20 场景视觉原型仍可显式运行，详见 docs/UI-PROTOTYPE.md。

## 开发运行

要求 Go 1.27.3。MyGo 与依赖已锁定。所有缓存、二进制、运行数据和截图必须位于仓库外，build root 必须已存在且为物理绝对路径。

```sh
mkdir -p /absolute/external/noonmark-build
export NOONMARK_MYGO_BUILD_ROOT=/absolute/external/noonmark-build
export NOONMARK_GO=/absolute/path/to/go
bash scripts/check
bash scripts/build
# v2-session-001 必须不存在；此入口只创建隔离 development 会话
"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -init-session "$NOONMARK_MYGO_BUILD_ROOT/v2-session-001"
# 关闭后重开同一会话
"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -session "$NOONMARK_MYGO_BUILD_ROOT/v2-session-001"
# 不接存储的旧视觉原型
"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -prototype
```

Linux 原生窗口需要 GTK3 和可用 DISPLAY / WAYLAND_DISPLAY。无显示服务器可通过 -render-one 在相同 MyGo 代码上生成离屏帧，但不算真实桌面验证。scripts/cross-build 编译 macOS / Windows / Linux 的 amd64 / arm64 六目标，不代表这些平台已运行。

## 模块与边界

- domain-core：TaskChain / Definition / Trace、精确时间与历史不变量。
- domain-extensions：分类、重复计划生命周期及版本历史，尚未接入运行 UI。
- domain-codec：候选 v7 规范编解码及安全校验，只用合成 fixture。Swift 互通尚未验证，非空分类 audit 默认拒绝。
- internal/taskapp：仅普通任务的开发会话事务与持久化，详细安全和容量边界见其 README。
- docs/migration-audit：完整产品差距与上游测试库存。

还未完成 SQLite schema 19、同步、AI、全局快捷键、子任务与完整编辑/分类/重复/飞光运行接线、原生时区/唤醒通知完整对等、真实一年互动演示及原生三平台验收。当前文件后端不能替代生产存储；不得打开、探测或迁移实际用户数据。

## 来源与许可

AGPL-3.0。上游 https://github.com/quboliu/noonmark，基线 43e6eefaae3a04be89cab3e234d470111afd80ef。MyGo 保留 MIT 许可。该独立仓库保留 ports/mygo 布局以便日后通过获准的上游 fork 分支提交 draft PR；目前未创建上游 PR。

开发会话格式已更新为 v2，以保留未分类任务也必须具有的分类历史事件。旧 v1 会话会明确拒绝打开，原资料不变；请另建全新 v2 开发会话，不执行生产迁移。
