# GO-CLASS-002：分类算法验证入口必须在写入前拒绝危险输出路径

状态：已修复（源码与本地门禁）。修复提交 db06aab109b4361cfd06f4c2ef10e405616414b4，Git author／committer 均为 Codex <codex@openai.com>，不以此推断自然人身份。远端写入待确认，尚未进入该提交的 GitHub CI 或发行。

## 症状、来源与影响

首次发现：2026-11-06（新加坡）。独立审阅固定 Unicode16 候选包时，合成路径实验确认验证脚本的输出保护存在三种误写：尾换行经 shell command substitution 去掉后，实际输出路径不同于校验路径；Linux 的双前导斜线路径与 pathlib 锚判断不一致；预置 hardlink 日志经 tee 改写同 inode 的另一名字。没有接触生产资料。

引入版本：尚未提交的独立候选，旧文件清单 SHA-256 为 73aacbaa29e0a0029c43f5dac9de728b6645976d7e5dfab2978f2b0bd6dfcc24，没有对应 Git commit。Git author／committer：不适用。可核验的操作来源是候选开发与独立审阅记录；真实自然人操作者未知，不以 Git identity 推断。正式仓库没有接入有问题的候选版本。

受影响的是开发验证脚本的输出、Go 缓存和证据文件路径，并非分类算法、应用运行资料或用户会话。仅限制模块内路径、只检查顶层 symlink，或把正向规范测试全绿当成目录保护证明，都不能排除这些错误。

## 根因与修复

- 首先只读校验原始绝对路径字节，拒绝控制字符、双前导／重复斜线、dot 组件和 symlink；保护整个源码仓库、模块聚合及相邻仓库。
- 校验后保留相同的环境变量字节，不用 shell command substitution 传递路径。构建目录、GOPATH 与 GOCACHE 必须两两独立且位于源码之外。
- 检查现有输出／缓存树，仅接受目录和 link count 为 1 的普通文件，拒绝 hardlink、symlink 与 FIFO 等特殊文件。
- 校验完成后才写入，并独占新建权限 0700 的随机 run 子目录；不复用或截断旧证据。校验器不是对抗同 UID 恶意并发替换或挂载变化的安全沙箱。

修复后的候选清单 SHA-256 为 b13a0d623a9ee4a903695265ce5668bad182a6ac2d42b12df8c037a2b67a0f31。算法、表、来源和规范测试 23 个文件在安全修复期间逐字节不变。

## 回归证据与永久门禁

独立审阅在修复前实际观察到上述三种误写，后续审阅没有再次运行旧破坏性复现；仅在新建合成 fixture 中测试修复。27 项拒绝负例在任何写入和 Go 调用前失败，两次正常外置调用建立不同 0700 run，前一证据与全部 canary、源码 bytes／mode／link count 保持不变。

永久入口是 TestCanonical16RejectsUnsafeVerificationPaths：从 Go 测试实际执行 scripts/test_guard_paths.py，再调用真实 verify.sh，检查返回码、拒绝原因、Go 未启动及文件前后状态。它同时承担 fast gate 与该开发工具实际使用路径的 symptom gate，已登记 gates.tsv，由 scripts/check 强制运行。另有 TestCanonical16RegeneratesPinnedTables 验证离线重生成；算法规范和全量六目标 probe 由同一聚合入口运行。源码模式检查不冒充真实路径验证。

本地完整 canonical16 验证和独立 ICU76／Unicode16 差分已通过。Swift oracle、正式仓库新提交的 GitHub CI 尚未执行；ICU 结果不等于 Swift 互通，目录保护结果也不代表 OS 实机验收。

## 发行与回滚

本包仍为未接线的独立候选，不开放非空 catalog，不改生产格式。没有发行受影响脚本，因此无用户资料迁移。若需撤回，撤回候选包及聚合入口即可，不删除输出／缓存或原资料。正式发行前必须固定修复 commit，并在目标 head 重新跑 CI；不得把源码备份或编译成功当成整个产品验收。
