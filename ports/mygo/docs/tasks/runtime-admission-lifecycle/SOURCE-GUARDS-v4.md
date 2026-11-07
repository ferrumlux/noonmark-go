# M1/M2 v4 源码守卫

2026-11-07。此记录说明源码冻结与测试守卫的静态合同；测试、编译、原生窗口及 CI 的执行结果须另行验证，不能由本文件推导通过。

- 产品完整集合由 v3 的 122 项到 v4 的 123 项，明确 8 个既有文件修改及 1 个新增 runtime_admission.go。PRODUCT-BEFORE-v4.tsv 与既有 v3 完整集合逐字节相同；PRODUCT-SOURCES-v4.tsv 来自冻结后的完整生产 Go/module 集合。PRODUCT-CHANGES-v4.tsv 只含这 9 个路径。
- 冻结历史集合仍为 73 项，6 个既有路径改变。FROZEN-BEFORE-v4.tsv 与 v3 AFTER 逐字节相同；FROZEN-PRODUCTION-OVERLAY-v4.tsv、FROZEN-AFTER-v4.tsv 明确绑定旧值、新值、完整路径与完整集合。
- v4 产品守卫核对完整输入、精确 delta、完整输出及当前完整实际集合，再逆变换到精确 v3，随后执行原 v3→v2→v1 守卫。历史守卫先执行既有完整链，再应用 v4，并核对完整新集合摘要及逆变换。新增测试覆盖每个 manifest 字节、每条实际路径的 hash／删除／别名、额外路径、空集合及格式错误。
- main.go 使用完整旧／新文件摘要门。MAIN-BEFORE-v4.go.txt 来自公开基线 c441d3b331dd3b3358d171f3c2d7e420bb9c2099 的精确 main.go，旧完整文件摘要为 a69e32cc56991f14a3e082f5e6cc6ee000cd7eeca2bdb9ee414a4cba0f693077。只有精确新文件通过，才交回精确旧文件，并继续原四所有者／三所有者检查；旧 runtimeExitMainOwnershipSHA256 和所有原 mutation 断言不变。
- 新 main AST 守卫独立核对启动失败的两段 drain、Run 返回后紧接 EndRun、活动 UI 等待、writer.Done、PreClose、dispatcher drain、Service.Close 顺序及原错误合并。main 的 ProtocolErrors／CompletionRejected 统计、协议错误合并及生命周期日志都属于精确字节与 AST 合同。每字节及显式生命周期变异测试另外保护新路线。
- 实际消费者明确使用 v4 输出作为旧守卫输入；AST 消费者链校验保留旧链并增加 v4，不是跳过或改写旧摘要。GUARD-SOURCES-v4.tsv 列出本轮 12 个守卫文件的精确摘要。
- 原脚本守卫及 38 个脚本／依赖文件已逐项静态核对，摘要全部不变。所有原守卫 const 声明保留。

## 行为边界

observeDay 第二跳 writer 拒绝只证明该次日期推进 worker 未获接纳，不能把已经发生的原 UI 准备（Finish、Observe，以及它们可能提出的独立保存）宣称为零 native 或零写入。此切片维持既有 UI 准备后入队顺序，不扩大为 Health、容量／FIFO 策略或异常退出草稿恢复。

## 摘要

- 产品 BEFORE：5109881f5dd9784dea5902470c52c9120e170a9393070bd21e93d477def25f01
- 产品 AFTER：01251fda3a9c54ad55a940f70d67713dbc16fd9e261ce585b57bd1a165f56a5d
- 历史 BEFORE：2b5491f5e2bf9d699a48a9ee7353a0cd013dc78f04f511ac9ba977ba9917a14d
- 历史 AFTER：15b7413a97a378f29f354c9f25c53adb4107972dfb168213add559729312b62d
- 当前完整 main.go：426fdf2864efefebbd0c54f9c09da91364a3b16b728ba2915524075a4da0cf48
