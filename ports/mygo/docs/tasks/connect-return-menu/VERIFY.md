# 普通任务回池菜单：真实接线与验证

本批在独立 `return-ui-work` 副本接入已有 `Service.ReturnToPool(chainID, traceID)`，不修改领域、审计、存储格式或核心批次文件。只使用外置 development 合成会话，不接 production、iCloud、Keychain 或 AI。

## 上游与视觉接点

上游 `43e6eef` 的 `DayTodoPage.swift:846` 为任务行 `.contextMenu`，`TaskContextMenu`（1236 起）按能力列出动作，回池使用 `AppCopy.swift:103` 的「回到任务池（留下轨迹）」。本批直接使用 MyGo `Element.ContextMenu`，沿用既有任务行 Radius(7)、布局和分隔线，不新增 toolbar、sheet 或伪造 undo。

今天 pending 且没有开放子任务时，菜单仅含已接通的「标记完成」和「回到任务池（留下轨迹）」；今天 completed 仅含既有「撤销完成」；未来 pending 仅含回池；历史无写入项。有开放子任务时只显示回池，完成圆圈同步禁用；锁日不显示写入项。回池本身没有 undo。系统原生菜单的字体、圆角和阴影由平台绘制，并非应用自造样式。Linux headless Tester 能验证原生菜单项目与选择回调，但不栅格化系统菜单 chrome；截图不能证明系统菜单圆角已跨平台验收。

## 安全链路

- 菜单仅在共享 titleHub 运行模式挂载，不提供同步写入后备路径。
- 菜单回调捕获不可变 chain+trace。`titleTransition` 使用原有原生最终化和标题导航排空协议，标题确认持久化后才进入 `commandOnce`。
- `return:<chain>:<trace>` 共享动作键去重两窗口及重复触发。领域命令仍通过共享 FIFO 执行，复用停止状态及自然日边界；没有跳过会话检查。
- 只读取已发布的深复制视图。磁盘锁内事务不阻塞 UI 帧，持久化完成但发布尚未送达时仍展示旧图。
- 新增窗口级单调 navigation 身份；原生菜单宿主 key 同时绑定窗口实例及 navigation，拒绝跨导航和同 UI 树重新挂接后的旧回调。成功回调只清理同窗口、同意图、同页面、同日期且仍匹配旧 trace 的选中态，连「离开后回到同一选择」也不会被旧完成覆盖。失败不清理选择；标题未保存时不执行回池并保留草稿。
- 原生菜单取消本身没有写入；取消已关闭菜单不能撤销已接受事务。另一任务新开的原生菜单不被旧完成关闭或改绑。

## 可执行门禁

`runtime_task_menu_test.go` 的 `TestReturnMenu*` 使用真实 MyGo Tester 与真实 Service 合成 fixture，覆盖：

1. 今天／未来菜单文案、取消、回池状态、投影与重开。
2. 标题已在途时再次输入、连续菜单双触发与两窗口去重。
3. 原生 marked selection 替换后、下一帧前调用动作，确认最终标题入盘才回池。
4. 旧回调抵达前的新导航、新同值选择，以及另一任务菜单的取消／重开。
5. 可重试磁盘失败，以及标题保存和回池事务各四阶段故障。
6. 跨日、失效 trace、错链、已完成／历史的菜单能力。
7. 独立审查原反例：旧菜单跨导航往返、同 trace 取消／重开后旧原生回调、内部模型的同 UI 树会话重挂。最后一项是防御级测试，不声称公开界面可执行此重挂流程。
8. 真正 Service 持锁暂停时重绘不阻塞，以及 FIFO 发布前无新事实泄露。

完整验收入口为 `scripts/check`（四模块 race/vet、依赖身份与原生边界门禁）、`scripts/cross-build`（六目标）和 `scripts/test-entrypoints`（真实 Linux development CLI）。输出放于显式外置 `NOONMARK_MYGO_BUILD_ROOT/evidence`，不把缓存、二进制、会话或 PNG 写入源码。

`return-day-before.png` 和 `return-pool-after.png` 为真实 1200×768 headless UI 输出，已检查。`return-native-menu.txt` 保存 Tester 查询到的真实原生菜单项目，不伪造菜单截图。真实显示服务器、macOS／Windows／Linux OS 输入法、系统原生菜单外观、Swift 动态 oracle 未验收。

## 回滚

UI 回滚不能转换或删除已写入的回池事实。原 Service 批次的严格 reader 范围与 v2 会话兼容性边界不变；旧 reader 可能拒绝新事实。保留原会话，不覆盖、迁移或修补未确认写入。

## 最终接合验证记录

2026-11-06（新加坡），Go 1.27.3 linux/amd64。应用与冻结菜单补丁接合后的 `TestIndependent*|TestReturnMenu*` race 通过（7.889s）；独立审核四条原反例与菜单／日视图 race 通过（6.850s），完整应用 race／vet 和原生门禁也独立通过。独立原始红／部分修复／最终绿日志均保留，不删除或放宽断言。

本工位完整 `scripts/check` 已通过：应用 main 61.888s、taskapp 16.537s，四模块 race／vet，依赖准备器 51 项，以及原生 UI 82 个顶层测试（2.763s）、root popup 结束门禁（1.060s）和 ui/root vet。门禁采用最新 `check-mygo-native`，包含 content.go 所在根 package 的 vet，不仅检查 ui。

接合的 manifest SHA256 为 `a218263cff8b2d6a8673b67c342dcf9ae3c111084f548f6709cea0220c6388c4`；本工位依赖 recipe 为 `61c70e9fc051e577ca983c7e5ebf0b3be02a46d7145cfad7ea4b2b8538cef19d`。不同源码路径有不同 recipe，这是正常重建，不能直接复用别工位 receipt。

最终六目标交叉编译全部通过（linux／windows／darwin × amd64／arm64），development CLI 的创建、重开、重复创建拒绝和未指定身份启动拒绝均通过，渲染 day／pool 各 2400×1536。记录分别为外置 `cross-build-final.log` 和 `cli-final.log`，不是补丁前预跑日志。交叉编译不等于目标系统运行验收。

额外完整框架 UI 全套的独立运行终态：1174 个顶层测试通过，仅既有 `TestDiffFrameAllocs` 失败（steady 3 / warm 9，对阈值 2，373.006s）。仅带原 0001／0002 的基线重跑同测试，也得到完全相同的 3／9（146.908s）。未改阈值、未跳过，不能宣称全框架全绿；记录见依赖说明的 `native-all-ui.log` 与 `baseline-diff-allocs.log`。
