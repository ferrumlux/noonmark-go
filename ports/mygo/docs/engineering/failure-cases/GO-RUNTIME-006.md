# GO-RUNTIME-006：标题规范等价输入错误创建版本

状态：开发候选红转绿，修复提交待本批登记；尚无标题 UI 和真实桌面门禁。

- 首次发现：2026-11-05，本批独立审查。引入位置是 connect-title-edit 未提交候选，基线 9e64c5749a951ffbca1a44347d3f758bea845d6c 尚无这两个标题入口，未发布受影响代码。
- 归因：没有引入提交，因此 Git author/committer 不适用。实际修改来源可由本任务的领域命令实施记录确认；不是用户编辑，也不能归因给基线提交作者。
- 症状：合成旧标题 café（U+00E9）输入 cafe+U+0301，原版 Swift String 比较应无操作，候选 Go 字节比较却在 rename 新增定义、重绑 pending，在 input 改变储存字节和时钟。无效时钟也可能使原本应忽略的 no-op 错误失败。
- 复现与证据：独立 overlay 对两个核心入口先复现失败；修复后同 overlay 和稳定 TestTitleCanonicalEquivalenceNoOp 转绿。没有用字体相似或 HasText 猜测，断言完整 graph、定义数量、身份及原字节不变。
- 排除假设：不是换行或 trim 问题，也不是磁盘写入问题；两个规范等价 Unicode 表示直接触发核心分支错误。
- 根本修复：以锁定 x/text 的 NFC 比较两边，保留持久化字节，不能用 NFKC 合并兼容字符。权限、日期和锁定检查仍在 no-op 之前，未使用的 clock/ID 不消费。
- 回归：双向组合/分解、组合字符顺序、Hangul、无效 clock/ID、不可编辑历史及兼容字符不同。Service 症状门禁验证两个入口无新增磁盘 revision、完整 snapshot 原字节不变；stale/blocked handle 仍拒绝。
- 门禁：gates.tsv 的 TestTitleCanonicalEquivalenceNoOp 与 TestTitleServiceCanonicalEquivalentNoop，scripts/check 强制运行 core 与 taskapp。前者为快速核心门禁，后者为当前实际 Service 保存路径；未冒称尚未实现的 UI 症状测试。
- 发行与回滚：未作生产切换。新 UI 不开放，六目标编译不等于各平台运行；本批提交正文引用本案例。若未来 Foundation oracle 揭示差异，必须新增明确 fixture 并修正规则，不能吞掉输入或强制改写旧字符串。
- 教训：跨语言 String 等价关系属于领域语义，不能用 Go 字节相等替代。引入 Unicode 库同时同步独立模块 manifest、最低工具链声明和整仓回归。
