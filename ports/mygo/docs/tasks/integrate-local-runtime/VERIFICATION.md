# 本地运行链路验证记录

日期：2026-11-05。只使用合成资料与新建隔离会话。

## 首批普通任务链路（4cbd568）

- scripts/check：原型与新运行包、三个独立领域模块的 race 测试、vet、gofmt 检查全部通过。
- 新运行快照完整测试：32 个顶层测试/入口、61 个 test pass events。UI 包 statement coverage 90.2%，internal/taskapp 76.5%。这不是原上游全部 1637 个测试的移植通过率。
- MyGo UI Tester：中文输入和 Enter、今日新增、任务池新增、排今天/明天、完成和撤销、日切换、导航、重复操作、取消、写入失败保留草稿、26 行滚动、快速记录、多窗口共享状态、关闭重开回读。
- 存储：全图往返与精确 Instant、倒退墙钟、并发 handle 冲突、进程内串行、外部修改、symlink、损坏、超大文件、半写入及 commit 前故障不发布、下次打开结算过去日期、不支持的 supplement 拒绝而非丢弃。
- 实际编译产物 CLI：列出 20 个场景、新建隔离 session、关闭重开同一 session、从同一 UI 代码离屏渲染、拒绝复用 Create 目录、拒绝无显式运行身份启动。
- 六个完整 App 目标编译通过：linux/amd64、linux/arm64、windows/amd64、windows/arm64、darwin/amd64、darwin/arm64。
- 原上游工作树保持干净；三批原始模块逐文件零漂移。

## 未通过或未运行

- 真实 Linux 桌面启动实际尝试失败：环境无 DISPLAY / WAYLAND_DISPLAY。不是 App 桌面验收通过。
- macOS / Windows 以及 arm64 没有实机运行；交叉编译不是原生运行证明。
- 没有 frontend-verify skill、Swift oracle、WindowServer/系统输入法、SQLite schema 19、iCloud、实时 AI 或一年正式互动演示验收。没有容器/部署入口。
- Windows 文件 fsync/readback 已实现；目录断电耐久性未验证，开发后端不能宣称生产级保障。
- 初次 race 编译因内存限制被系统终止；改用单包构建、GOGC=20、GOMEMLIMIT=700MiB 后完整复跑通过，脚本已固定这些保守默认值。

全部新日志、二进制、会话与截图存放在源码外部 build root。此文件只记录可复现结果与限制，不包含生产资料或凭证。可用脚本重复检查；原型图片与运行图片分别生成，不混作完整功能对等证明。

## 聚合审计及自然日修复（GO-RUNTIME-003）

2026-11-05：在 f747dc4 完整性修复基础上，以新 v2 development 身份复跑 scripts/check、scripts/cross-build、scripts/test-entrypoints 全部通过。原型与运行包以及三个领域模块 race/vet/gofmt 均通过；六目标完整 App 重新编译通过，CLI 创建、重开及拒绝复用验证通过。这些证据仍不是非 Linux 实机或 Swift oracle 证明。

- 服务持有完整候选 snapshot 与任务图，同一事务结算任务、生成空分类 audit、严格验证及持久化，成功后才成对发布。
- 精确验证已结算 completed/unfinished 的事件、latest snapshot、revision、引用及 LockedAt/SettledAt。拒绝零化 audit、已有历史重建、非空 catalog 与未接通 supplement。
- 存储故障注入证明图、audit、today 均不提前发布；独立检查包含连续 12 次跨日、倒退墙钟、每次重开字节一致、写后失败重开不重复事件。
- MyGo 控件回归证明自然日推进发生在命令前，跟随今天的窗口同步前进，明确选中的历史日期保留；失败保留草稿并阻断后续操作。
- 旧 v1 development 会话明确拒绝，目录不变。不自动修复、迁移或删除，应另建新 v2 会话。
- 每分钟轮询及命令前检查已经接通；操作系统唤醒、时区与 locale 通知尚未完成原生验证。视觉源码常量校正属于后续独立批次。
