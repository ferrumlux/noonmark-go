# 来源与许可

本目录是 Noonmark 输入草稿自动保存 gate 的 Go 改编，使用 GNU Affero General Public License version 3；完整条款见 `LICENSE`。保留上游的权利与来源，不将改编声称为独立原创算法。

- 上游：<https://github.com/quboliu/noonmark>
- 精确 commit：`43e6eefaae3a04be89cab3e234d470111afd80ef`
- 源实现：<https://github.com/quboliu/noonmark/blob/43e6eefaae3a04be89cab3e234d470111afd80ef/Sources/NoonmarkMacRuntime/IMETextDraftAutosaveGate.swift>
- 官方测试：<https://github.com/quboliu/noonmark/blob/43e6eefaae3a04be89cab3e234d470111afd80ef/Tests/NoonmarkMacRuntimeTests/IMETextDraftAutosaveGateTests.swift>
- 本次改编：2026-11-05，Swift 状态机转为 Go，增加构造错误与零值 fail-closed 防护，增加源码分支与边界回归测试。

本次没有运行 Swift/XCTest 或原生 IME；来源对应不代表已经有跨引擎动态对账证据。

Unicode canonical comparison 使用 `golang.org/x/text v0.43.0` 的 `unicode/norm`。该依赖由 Go module 引用，未将第三方源码复制进本目录；其许可随依赖模块保留。依赖版本与正式 Go 移植模块一致。
