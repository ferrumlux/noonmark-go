# 隔离任务应用服务

当前服务只接入第一批 TaskKernel 的普通任务：任务池新增、今日新增、今天/未来排期、今日完成/撤销、两个标题保存入口及重开时的过去日期结算。重复计划、分类、飞光、子任务编辑与完整上游命令还未接入此运行服务。

## 提交边界

每个命令在完整 task + classification history clone 上执行，经过领域验证、无条件 empty-history audit 与候选 v7 编解码规范化，再写入新 revision。标题 no-op 不新增 revision，但仍检查既有磁盘 prefix，不能绕过损坏或冲突检查。整个写入和回读确认成功后才替换内存快照。UI 不乐观发布未保存事实。重复完成、过去/未来完成、无效标题及错误排期沿用领域拒绝条件。

磁盘目录必须由 Create 以 exclusive mkdir 新建，并带固定 development marker。Open 只接受该身份，不访问默认用户目录、production、iCloud 或 Keychain。路径及祖先 symlink 被拒绝；读写使用 os.Root 限制目录边界，文件还须为普通文件并保持读取前后身份一致。调用者应传入物理绝对路径。

每次提交建立新的 rev-N 目录，state.json 使用 O_EXCL，文件 fsync 后逐字节回读，再创建同样经 fsync 与回读的 COMMITTED hash。revision 与父 hash 连成连续链。竞争写入只有一个进程能获得下一 revision；另一进程收到冲突，不覆盖他人状态。半写入不会静默跳过或修复，保存原资料并停止后续修改。hash 用于损坏检测，不是对抗同账号恶意篡改的签名。

最多 1024 revisions、每条 24 MiB、全会话 128 MiB；超限拒绝新写入，不删除历史。这是有限开发后端，不是 SQLite schema 19，也不是生产级长期存储。Windows 已强制文件 fsync/readback，但目录 fsync 无可移植 Go 保证，需要未来原生后端及断电验证；不得宣称跨平台断电耐久性已完成。macOS / Windows 尚无实机测试。

## 传输格式边界

外层 format=noonmark-mygo-development-v2，payload 为第三批候选 v7 bytes。只接受空 catalog（无分类/标签项）的完整结算历史、无 Idea、无 recurring series 和固定关闭同步的偏好，不会静默丢弃未支持的 supplement。每个锁定的 completed/unfinished trace 必须有对应空分类事件与 latest snapshot，revision、身份、状态、捕获时间严格校验。即使内容符合某些 v7 形状，这也不是面向 Swift 的正式导入器。Swift oracle 尚未验证。

空 slice 在 wire 上规范化为 []。服务在首次发布与每次提交前统一规范化，因此进程内快照与重开结果一致；Instant UInt64 bit pattern 原样保留。

跨日结算与空分类历史在一个候选快照中验证和提交。打开、每分钟轮询及每次命令前检查自然日；失败不发布任务、audit 或 today 任一部分。手动历史选择保留；尚未实现/验证原版全部系统时区、唤醒和 locale 通知。没有一年演示故事、SQLite 或加密 AI sidecar，因此不能称为上游正式互动演示基线。

旧 v1 development session 不包含完整结算 audit，因此按开发 clean-cut 拒绝打开而不迁移，原文件全部保留。请创建全新 v2 session。通用 v7 codec 本身仍默认拒绝非空 audit；当前 runtime 的审计函数只证明受限空 catalog 子集，任何更丰富分类事实仍 fail-closed。

## 标题编辑的专用保护

`RenameTaskTitle` 与 `SaveTaskTitleInput` 区分显式版本分叉和输入复用，调用方不能用 rename 代替逐字 autosave。标题命令使用独立重放审计，严格冻结历史内容，只容许当前历史定义向本次合法新后继推进三项 metadata。原 `CaptureEmptyHistory` 完全不变；缺失 audit、更多 runtime scope、任意字段豁免均拒绝。详见 `../../docs/tasks/connect-title-edit/TRANSACTION.md`。

Swift String canonical 等价输入不重写原字符串或消耗磁盘 revision。Service 仍在私有候选中预计算时钟和身份，不能把这个入口理解为不消耗任何计算资源；核心 no-op 本身不读取未使用 clock/ID。日待办标题 UI 已通过 titlesession/titlecoordinator 接入非空约束、IME、700ms debounce、失焦及可取消退出；核心允许空标题不代表 UI 将空白持久化。原生实机与 Swift oracle 仍未验证，详见 ../../docs/tasks/connect-title-autosave/NATIVE-UI.md。

`SaveTraceTitleInput(traceID,title)` 为详情输入提供同锁 trace 路由，先拒绝过期或非 pending 的所选事实，再进行相同标题事务，避免历史回调借同链新 pending 写入当前标题。实际编辑 UI 使用同一事务返回已核实字节的 `SaveTraceTitleInputAcknowledged`，不会拿稍后 Snapshot 或请求字节冒充保存确认。
