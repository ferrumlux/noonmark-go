# C1：受控延迟关闭局部修复

本开发版本临时停止保存完成后自动重试 Quit。即使没有快速记录窗口，退出请求因保存未完成而被拒绝后，也需要用户再次明确退出。这是有意的临时 UX 差异，不代表与 Swift 原版完整对等。

快速记录（quick-entry / flylight-quick-capture）的 Close、取消与 Escape 仅代表当次关闭请求。被安全门拒绝后，已接纳的保存继续，但旧保存完成不能再次关闭窗口，因而不会用旧关闭请求丢弃后来已经交付到绑定的输入。非快速记录窗口仍保留原延迟 Close。普通保存的文本清理规则未变。真实 Service／输入回归只覆盖已接通的 quick-entry；flylight-quick-capture 在此仅按既有 utility 分类防御性排除自动关闭，不代表它的运行功能已接通。

## 最小实现合同

- coordinator 的 BeforeQuitOnce、BeforeCloseOnce、ReleaseClosedOnce 在意图创建时即禁止自动通知，并在返回时只取消自己持有的意图。同步重入创建的新意图不会被旧清理删除。
- 新明确请求重新执行 finalize、pump、clean 与真实 disposeProof。取消退出不清 session、flight、timer 或保存错误，也不伪造 ack。
- runtime 从构造起禁用 coordinator 自动 Quit，并移除 command 与 category 的自动 Quit 出口。真实用户 Quit 继续经过全部原安全门。
- command、category 与 coordinator 自动 Close 均只允许仍登记的原窗口且它不是快速记录。coordinator 策略在创建意图时捕获原窗口身份，通知前核对，并复验原意图及注册证明。CloseRetryGuard 的 factory 与返回 predicate 都只能纯读取，不得调用公开 API、外部 hook 或同步重入；此接口不保证任意 hook 重入安全。
- command / category 的真实 ticket、receipt 与错误结算保留；失效窗口不会借旧完成关闭新窗口。

## 非关闭行为

runtime 不再持久设置 quitting 等待位。成功 carryover 的跳转、clean/locked 标题退休因此不再只被旧退出请求抑制，但原 current-owner、navigation、RetirementIdle、native、composer、dirty、flight 与 error 安全门全部保留。非快速记录的 closing 等待位及其抑制作用仍保留。

## 验证边界

测试使用隔离开发资料、真实 Service 和 UI Tester 绑定，并单独测试 coordinator 的同步重入、第二阶段关闭、注册变化及真实保存。Tester 的关闭或 Quit 回调计数不是操作系统窗口关闭证据。完整门禁、race、跨平台构建与三平台原生事件须分别记录，不能由这份合同视为通过。

本切片不解决未 drain 的原生输入、ABA、IME、异常退出后的草稿恢复或持久化恢复；不扩展 MyGo、Health、undo 或生产准入。AGPL-3.0-only 授权维持不变。
