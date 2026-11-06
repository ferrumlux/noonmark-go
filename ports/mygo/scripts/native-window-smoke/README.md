# 真实 Day 窗口生命周期门禁（第 1 层）

这是隔离 development session 的受支持外置 driver，不是生产入口、桌面自动化或像素验收。固定 `display_pixels_verified=false`，不验证输入、IME、compositor 或硬件 GPU。合成 parser/进程测试不能作为原生平台通过记录。

## 入口

```sh
python3 -B ports/mygo/scripts/native-window-smoke/run.py \
  --artifact /absolute/external/artifact \
  --target linux/amd64 --source FULL_40_HEX_SHA \
  --run-id BUILD_RUN --run-attempt BUILD_ATTEMPT \
  --consumer-attempt CONSUMER_ATTEMPT --artifact-id EXACT_ARTIFACT_ID \
  --work-parent /absolute/external/runtime-parent
```

复用 `native-smoke/smoke.py` 的 artifact 白名单、二进制格式/hash、source/run/attempt 及 native host 检查。driver 不构建、下载或接收任意命令、session、report、work-root 参数。work-parent 必须已经存在、无链接/reparse、位于本 checkout 与 artifact 外；调用方还须保证不在其他源码或 prepared dependency 内。它是外置 build/runtime 根，不是任意用户资料目录。driver 不扫描整机寻找其他 checkout。

每轮独占创建 private work-root（POSIX 0700）。固定两进程依次调用真实产品：

1. `-init-session ROOT/session -screen day -native-window-smoke ROOT/stage1.json`
2. `-session ROOT/session -screen day -native-window-smoke ROOT/stage2.json`

第二阶段仅在第一阶段完整成功后启动，同 canonical path、development v2 身份与有界提交清单/hash 必须不变。日期变化返回 `date-boundary-not-covered`，不重建、不重试。手动运行产品 CLI 不具有 driver 合成来源保证。这些检查不是 OS sandbox 或恶意同用户认证。

## 平台状态

- Linux：实现自有前台 Xvfb、前台 dbus-daemon、产品三个独立 session/process group。随机预选 display、私有 Xauthority、`-displayfd` 必须返回同一号；不接受已有桌面、不使用 `-ac`。先前独立实验只启动 Xvfb，即在 listener 准备时失败；dbus/app 未启动。参数组合与真实窗口仍待官方 host 实测。
- macOS：使用自有进程 session，需实际登录桌面。此实现尚未取得 macOS 原生窗口通过记录。
- Windows：已实施窄 Win32 适配器，但**实际 Windows API 与桌面行为尚未实测**。先配置本轮 `KILL_ON_JOB_CLOSE` Job，用 `CreateProcessW(CREATE_SUSPENDED)` 暂停创建产品，成功 Assign 后才 Resume。`STARTUPINFOEX` 仅允许 stdin/stdout/stderr 三个句柄继承；使用明确 exe、Windows argv quoting、受控 Unicode 环境块和 cwd。Assign/Resume 失败时终止并有界收割本轮进程，不使用 breakaway、无 Job 降级或修改外层 Job/系统权限。收割先确认进程结束与 Job active-process 清零，再关闭 Job；无法确认明确 `cleanup-incomplete`。Linux 上的注入式 API 顺序/失败/句柄测试不是 Windows 平台通过。

driver 本身不安装软件，不修改 MyGo 或 OS 权限。独立窗口 CI 只在官方普通 runner 安装固定 Linux 依赖。任一 helper 缺失、display 冲突、readiness mismatch、非本地 dbus address 或 helper 提前退出均失败。

## 预算、报告和失败证据

总预算 120 秒；每阶段最多 60 秒，第一阶段包括 artifact 前置校验。45 秒内按字节读取到完整精确 READY 行（LF 或单 CRLF），最多一次；随后取 `min(stage+55s, marker+10s)` 验证自然 exit 0、完整严格 JSON 与 session。内部 request→finalized 也须不超过 10 秒。最多余 5 秒 TERM→KILL→wait/reap 本轮持有进程组；任何 deadline/日志超量先锁定失败，之后收到成功报告不能翻绿。子进程退出后仍有界消费 stdout/stderr 到 EOF，以捕获残留重复 marker。

这是软件 watchdog，不是硬 OS 时间/CPU/磁盘配额。同步 Popen、文件 I/O 或不可中断 OS wait 可能不能被 Python 抢占；外层 job 超时下未返回结果须算失败/未知。`reaped` 与 `group_empty` 单独记录，发出 kill 不等于成功收割。

schema 固定包含实际 today、唯一窗口、有限顺序事件及相对纳秒、native frame 到达、可见性/handle 存在/尺寸、真实 Run 返回、两 lane drain、首次 Service.Close 结果、controller 停止。拒绝缺字段、未知字段、重复 JSON key、非有限或负时间、错误基数/顺序、尾随内容与大于 64 KiB 报告。READY 仅供阶段计时，不替代最终报告；报告 complete 不能自证 fd 已关闭，必须外部 exit 0。

证据仅取 `manifest.json`、`result.json`、`stage[12].json`、`stage[12]-driver.json`、stage stdout/stderr 和必要的 helper 有界日志；每份日志上限 1 MiB。**不要上传整个 work-root、session、HOME/cache 或 Xauthority。** 私有 cookie 不进入日志/参数或证据。没有 PNG 或屏幕像素。

## 合成测试

```sh
python3 -B -m unittest discover -s ports/mygo/scripts/native-window-smoke -v
```

覆盖严格 schema/cleanup 缺失/首次 Close 错误、事件顺序/overflow/时间、完整 marker 拆包/CRLF/重复/尾部/伪子串、stderr 不算 marker、私有路径/环境、固定 helper argv、缺 helper/Windows API 失败无非托管启动、真实 POSIX 子进程有界收割、超时后失败不可逆、两阶段同路径及日期/清单变化。测试中的合成结果不是真实窗口通过记录。

Windows 适配器的管道用 `PeekNamedPipe` 检查，再读取已可用的有界字节，不启动可能永久阻塞的 reader thread；正常退出后仍在同一预算内读到 EOF。Job/process/thread/pipe 所有权只属于本轮，失败后仍尝试关闭其余句柄；`CloseHandle` 或观察失败不会被当作完整收割。

## 独立 CI 与封闭证据

`native-window-smoke.yml` 与原 CLI/headless `native-smoke.yml` 分开，不替换或削弱原门禁。六个官方普通 runner 只验证 lifecycle-only。build outputs 给出精确 source、run、build_attempt 与 artifact ID；consumer_attempt 记录执行轮次，重跑 consumer 不把旧构建写成新构建。新参数必填，不从 manifest 推导预期身份。

`collect.py` 只接受一次新建 parent 下唯一 private root，并逐一复制固定 manifest/result、两个 stage-driver 与最终 app JSON、固定 app/Xvfb/dbus stdout/stderr。JSON 每份最多64KiB（manifest 4KiB），日志每份1MiB；超量只保留原始前缀并记录 truncated，链接拒绝。失败、空或截断 JSON 原样保留，不合成成功报告。不会递归上传 session/HOME/cache、二进制、Xauthority/cookie、环境或屏幕。输出只上传明确列出的固定路径。

collector 成功仅代表收集完成；独立 `--gate` 必须重新用 trusted identity 调用原 smoke.verify，核对实际 binary hash、manifest、result、每阶段 exit/marker/report/cleanup、两个会话快照与 workflow outcomes；缺失、额外、超量、错误来源或尝试号均失败。上传成功不能使运行失败变绿。校验依赖受审 workflow 和二进制，不是恶意同用户攻击下的认证。

本批仅合成测试和静态契约测试。六平台真实窗口仍 not-run；先前 Linux 尝试只在 Xvfb listener 失败，dbus/app 未启动。Windows nested Job 与 macOS GUI 能力必须由后续官方 host 实证，不能自动重试、breakaway 或修改权限绕过。
