# Native runtime smoke

此入口只验证已构建产物的真实主机 CLI、离屏 PNG、初始 development 会话创建／重开。不构建，不下载，不访问账号，不验证窗口或 IME。

`smoke.py pack --binary ABS_BINARY --artifact NEW_ABS_ARTIFACT --target linux/amd64 --source FULL_SHA --run-id RUN --run-attempt ATTEMPT`

`smoke.py verify --artifact ABS_ARTIFACT --target linux/amd64 --source FULL_SHA --run-id RUN --run-attempt ATTEMPT`

`smoke.py run --artifact ABS_ARTIFACT --target linux/amd64 --source FULL_SHA --run-id RUN --run-attempt ATTEMPT --work-parent EXISTING_EXTERNAL_TEMP`

使用 Python 标准库，Python 3.9+。消费端不 source environment、不需 Go。Linux 运行前须由独立准备阶段提供 GTK/Pango/Cairo、fontconfig/CJK 字体及 EGL/Mesa 等原生动态库；离屏软件渲染可显式设置 LIBGL_ALWAYS_SOFTWARE=1、EGL_PLATFORM=surfaceless。

每目标 artifact 精确两个文件：目标 exe 与 manifest.json。由固定 workflow 在本次 build 后 pack，消费者仅下载同 workflow/run 的确切目标 artifact；调用参数 source/run/attempt 必须来自可信 job 上下文，不能读 manifest 回填预期值。清单是错配／损坏检查，不是签名或独立来源证明。pack 不创建 zip，服务端传输解包只到全新隔离目录；不从 artifact 执行任何脚本。

run 校验 OS 原生架构及 ELF/PE/Mach-O 文件头后仅为已校验 exe 恢复 POSIX 执行位。脚本来自受审 checkout。QEMU/container 等非原生执行环境不得由调度方冒充真实 runner；本脚本不是虚拟化检测器。输入 artifact 和 work-parent 必须是执行方新建且独占控制的临时路径，不接受用户提供的共享／并发可变目录。

每次运行在 work-parent 内新建目录，不复用／删除 session。保留阶段 stdout/stderr、退出码、耗时、PNG 与 result.json。失败不自动清理。仅将有界日志、PNG、result 和 manifest 加入后续明确上传清单，不上传 session、缓存、源码或整个 work-parent。进程日志轮询上限 1 MiB，20ms 检查；应用失控瞬间可能超出上限后被终止，不宣称操作系统硬配额。

fixture：`python3 -B ports/mygo/scripts/native-smoke/test_smoke.py`。合成可执行头／PNG／假调用仅验证驱动 fail-closed，不能证明平台实际运行。真实结果需要独立 native run。

## macOS 架构判定

通过系统 libSystem 的 `sysctlbyname` 查询固定能力键并保存返回码、errno、长度和值，只有明确 ENOENT 才视为可选键不存在。ARM 能力缺失本身不能判定 Intel；还要求正向 x86_64 能力以及进程架构／Rosetta 状态一致。Apple Silicon 上翻译运行的 x86 Python 可被识别，但待运行产品仍必须是与原生主机匹配的 arm64 Mach-O。查询异常或矛盾保持失败，`stage.json` 保留失败前的诊断，不收集全环境。

首轮 [42277984531](https://github.com/ferrumlux/noonmark-go/actions/runs/42277984531) 的 Intel 检查器在应用启动前收到 sysctl 非零退出；旧代码没有保留该子进程的输出与确切退出码，所以不能推断具体系统错误。本修改的合成 ctypes 测试不替代修复后 macOS 的实际运行验收。
