# GO-NATIVE-HOST-001：Intel macOS 架构预检阻断真实运行

状态：修复已合入，**Intel macOS 原始症状已通过真实运行验收；ARM macOS 两次未取得 runner，双 Darwin 验收仍不完整**。关联 #12，完整桌面仍见 #9。未发行，未接触生产资料。

## 原始症状与证据

引入正式提交为 [0a680da10c4119d48ca6176e9efcbf8b6584dcef](https://github.com/ferrumlux/noonmark-go/commit/0a680da10c4119d48ca6176e9efcbf8b6584dcef)，首轮 [run 42277984531](https://github.com/ferrumlux/noonmark-go/actions/runs/42277984531) / Intel job 128102410317 在 `consume.py` 的 native-host 阶段失败。manifest 校验已完成，应用 CLI、会话创建与 PNG 尚未执行。原驱动对 `/usr/sbin/sysctl -n hw.optional.arm64` 的非零退出直接抛出 `native macOS architecture query failed`。

首次可核失败时间：2026-11-06 19:21:57 UTC，以上 hosted job 日志。引入提交 Git author 与 committer 均为 `ferrumlux <noxlimen@tutamail.com>`，提交时间为2026-11-06 13:12:02 -06:00；实际自然人操作者证据不足，不作归因。

原件保存了 macOS 15.7.9、CPython 3.14.7、Python process machine=x86_64 和失败栈；未保存该 sysctl 子进程的 stdout/stderr/确切退出码，**不能推断真实原因是 ENOENT 或 unknown oid**。这不是已证明的应用运行故障。Git 身份不是自然人归因依据，实际操作者不据提交身份推定。

## 修复机制

旧检查要求每台 Mac 都成功返回 ARM 可选能力键的 0/1；对于明确不存在该键的 Intel 系统没有合法分支，其他错误也缺诊断。修复使用固定系统 libSystem 的 typed `sysctlbyname`，保留返回码、errno、int 长度和值。仅 `-1 + ENOENT` 代表可选键不存在；权限、I/O、格式及未知错误一律拒绝。没有使用会吞掉名称查询错误的 `sysctl -i` 捷径。

缺少 ARM 能力本身不能推出 Intel：还须正向 `hw.optional.x86_64=1`，Python 为 x86_64 且无翻译状态冲突。Apple Silicon 的 x86 Python 必须具有 Rosetta 标记，主机仍识别 arm64；执行产物必须保持匹配的 arm64 Mach-O，不接受 amd64 产物冒充原生。未知或矛盾组合不运行应用。固定 query 记录在成功和失败的 `stage.json` 中保留，不收集全环境。

## 永久门禁与当前验证

`native-smoke-gates.tsv` 单独登记 Python／真实 host 门禁，不混入只登记 Go `scripts/check` 的既有 registry。默认 `.github/workflows/native-smoke.yml` main push 入口先运行全部 `test_*.py`，其必需 fixture、逐目标产物绑定、失败不转绿及 evidence gate 均有契约负例。

- Fast gate：`Fixtures.test_mac_intel_requires_positive_capability`、`test_review_mac_query_fail_closed`、`test_mac_arm_and_rosetta_not_mislabelled_as_intel`、`test_mac_query_failure_keeps_errno_receipt` 及 consumer 失败保留测试。
- 原症状 gate：修复后精确 commit 的 Intel 和 ARM macOS runtime job 必须实际通过来源校验、架构检测、CLI、Day/Pool 渲染、隔离会话及 required evidence gate；不能以 Linux 模拟、unit fixture 或跳过 job 代替。
- 初始本地24项 native fixture、既有 CI contract、release 17单元和17 workflow负例通过。独审另以真实 Linux ctypes 调用自编 C shim 测试89次场景，10条 consumer/collector 路径、6个吞错／弱化守卫变异均正确拒绝。这些不是真实 macOS 或 Rosetta 证明。
- 增补本故障 registry 自检后重新执行全部 fixture：25项、0.509秒、退出0。

回滚只撤销检查器和对应诊断，不改产品、会话或发布权限；旧 Intel 预检仍可能阻断运行。首轮证据保留，后续新 run 不覆盖旧失败。教训：可选平台能力不能靠一次命令成败二分推断，未知错误必须保留原始诊断，支持分支必须具有正向证据。

## 2026-11-06 精确修复版本的实际证据

固定修复 [a3e2a407](https://github.com/ferrumlux/noonmark-go/commit/a3e2a40797bf41bc5762ded58f72fdfa5d51dbce) 的[开发 CI 42282151849](https://github.com/ferrumlux/noonmark-go/actions/runs/42282151849) 已成功，完整日志、官方 artifact 的实际字节、CRC、六成员及来源身份均经复核。root 为473.077秒，原600秒上限不变；这是单次观测，不保证后续性能。精确摘要与哈希见 [GO-NATIVE-HOST-001-EVIDENCE.json](GO-NATIVE-HOST-001-EVIDENCE.json)。

[Native run 42282152002 / attempt 1](https://github.com/ferrumlux/noonmark-go/actions/runs/42282152002/attempts/1) 的 build 与 Linux/Windows 两架构、Intel Mac 五个 runtime 均成功，并核对12个官方 ZIP、10张合成PNG和6份完整job日志。Intel job 128116713487 实际观测两个可选键均 return=-1、errno=ENOENT、size=4，而 hw.optional.x86_64 为 return=0、errno=0、size=4、value=1；随后 CLI、创建/重开、拒绝护栏、Day/Pool 2400×1536渲染及必需 evidence gate 均通过。这证实新路径，不恢复旧 sysctl 命令未保存的 errno。

ARM Mac attempt 1 的 job128116713590及唯一一次单job重试 attempt 2 的 job128125924170均 cancelled，runner_id=0、steps为空，没有应用执行或ARM runtime产物。两次独立官方job页分别显示服务器错误、未获hosted runner及ARM容量提示；不推定计费、权限或取消操作者。Attempt 2 中其余五个成功job只是旧结果重映射，完整起止/步骤时间戳一致，不计作第二次执行。已停止循环重跑，#12保留ARM runner受阻；这不是已执行的产品故障。

这些结果不包含实际桌面窗口、系统像素回读、IME、Rosetta实测、生产资料、安装签名或发行。完整#9仍开放；后续产品批次需继续保留六目标实际运行门禁，不能以模拟或编译替补未运行的ARM Mac。
