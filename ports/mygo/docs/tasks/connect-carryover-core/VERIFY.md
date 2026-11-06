# 延期与延续核心整合验证

task-id: connect-carryover-core

## 需求、现状与设计

用户要求完整跨平台复刻。此批是对上游 43e6eef 的普通任务延期与历史延续领域命令移植，整合基线 ddee25c；接口和风险见 DESIGN.md。只接入 TaskKernel，不让未经事务审计的事实进入磁盘或 UI。原上游与框架仓库未修改。

## 冻结及独立审查

三份作者文件在所有最终测试前冻结：carryover.go 的 SHA256 为 8e18447e5db674a4a0922ef14834c0c8a5b576f0d32424310c77dbee3ded7dd1；carryover_test.go 为 c1bef356a8b5f042cbfe182de20c810b16b7f0a74d1f53a1dcc4076757d03df6；DESIGN.md 为 f28ab4e07b2ed06b13f4e1b5faf169c3fbb2f199748a96030bd10f08ee6f7239。

另整合独立审查 carryover_recovery_independent_test.go，SHA256 为 0e6307d1025e96e2fa5b8feaa240399d5a7e72043e79c16f3a43f1e81128208c。覆盖确定性身份映射、负数／小数及相等时钟、来源历史不改写、后期子项失败的整图原子性、活跃轨迹／池位置拒绝、取消历史排除，以及既有日期不消耗新身份。

早期红色 fixture 及纠正记录保留在外置证据；没有绕过 validator。序号最大值防溢出分支没有可行大小的完整有效链动态覆盖，不宣称命中。未确认此批引入产品故障。

## 正式副本验证

四份 Go／设计文件先全部复制到正式副本，哈希核对不变，再运行以下入口。2026-11-05 UTC 的结果如下：

- scripts/check：退出 0。根 UI race 199.091 秒，taskapp 34.908 秒，taskview 1.057 秒，core 9.906 秒。四模块 race、vet、格式检查、完整 Unicode 16 门禁、28 个合成 Swift 比较工具测试、51 个框架依赖回归及原生 popup 合同均通过。
- scripts/cross-build：六个目标 linux、windows、darwin 的 amd64／arm64 全部编译成功，退出 0。这不证明六平台实机运行。
- scripts/test-entrypoints：实际 Linux 隔离开发身份的创建、重开及错误身份／覆写拒绝通过；Day 与 Pool 各渲染 2400×1536 MyGo 像素，退出 0。没有桌面 WindowServer、IME 或 OS 窗口验收。
- 最终再次核对四份文件哈希及 git diff --check。只有本验证文档在上述运行完成后新增。

证据位于显式外置构建根的 noonmark-carryover-build/integrated-check.log、integrated-cross.log、integrated-cli.log 及 recovery-report.md。没有生产数据或真实同步资料参与。

## 范围、回滚与下一步

Service 的 exact replay、空 catalog 分类快照／事件与故障原子性仍由下一批独立验证。撤回延期、完整 undo、日期选择器、完成／未完成完整页面与分类 Swift oracle 不在此批。核心成功不等于可持久化、UI 可用或产品完成。

本批只新增纯领域命令及测试，回滚可移除这些新增文件；不得为旧 reader 删除已持久事实。GitHub 写入仍等待用户确认，故只有本地提交与完整 Git bundle，没有对应新远端 CI，也没有发版。
