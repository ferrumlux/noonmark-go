# 分类 UI 精确源码冻结第八阶段

## 已审字节与许可范围

本阶段在普通分类 Service 第七阶段之后，明确追加 `categoryUIFrozenOverlay`，只许可以下四件原有 UI 源码的固定旧／新 SHA256：

- `runtime_carryover_actions.go`：自动退出当前编辑对象前检查分类草稿及在途状态。
- `runtime_day.go`：真实日详情分类落点与受保护回池详情的有限保留。
- `runtime_subtask.go`：任务详情分类入口及原详情交互衔接。
- `runtime_title.go`：同一 writer lane、导航／关闭屏障与回执处理接点。

这些是已审分类编辑及修复两段的完整字节，不是重新开放这四条路径任意修改。新增 `runtime_category.go` 不属于原73件冻结生产清单；本许可没有把新增文件混入旧清单。默认启动没有调用 `enableCategoryCandidate`，本批不开放生产或默认非空分类入口。

固定组合 `96665d5be5edc27788ac6bb678711364f18229ea` 的初始 focused 曾因这四件与旧冻结链不同而真实失败，首个诊断为 `runtime_carryover_actions.go`。这不是产品语义失败，也不能忽略门禁；原失败保留，本阶段显式补齐受审源码许可。

## 不放宽旧合同

`FROZEN-BEFORE-v1.tsv` 为第七阶段后的完整73件集合，`FROZEN-PRODUCTION-OVERLAY-v1.tsv` 为精确四行 old/new/path，`FROZEN-AFTER-v1.tsv` 为第八阶段后的完整73件集合。其余69件 hash 不变。旧七阶段 manifests、校验函数、拒绝测试、生产 hash 循环与后缀 hash 不改。

消费者显式读取并调用第八阶段，将结果赋回 `expected`，随后执行原生产 hash 循环。旧两处 consumer-chain 期望列表仅尾加第八项，原七项与顺序保持。新增专属 AST 检查锁定调用顺序、输入／输出赋值及早于 hash 循环的位置。

新阶段验证完整73件输入集合的固定摘要与规范形状，并要求四行 overlay 逐字一致。缺失、重复、额外、alias、错 old/new、提前推进、非规范行尾、字段数错误均拒绝；成功才复制结果，失败返回 nil 且不改输入。新增测试包括逐字节扰动及73件基线逐项删除／换 hash／alias，不以动态重算当前源码生成期望。

## 检查边界

本阶段只新增源码范围许可、tests/docs，不改任何产品文件、模块或默认入口。其检查不代替完整 `scripts/check`、跨编译、entrypoints、真实 native/IME、Swift 完整状态或生产验收。完整验收必须针对后续固定组合另行执行，不可用此前不同 head 的有限通过汇总冒称通过。
