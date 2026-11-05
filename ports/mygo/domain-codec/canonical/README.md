# 有界 JSON 安全边界

本包只实现 v7 codec 所需的 JSON 基础层，不实施领域校验，也不读取或写入生产资料。

- `Parse`：检查输入字节数、深度及节点数；拒绝各层重复的解码后 object key、非法 UTF-8、未配对的 UTF-16 surrogate、尾随内容和非法 JSON。数字以原始十进制文本保存，不经过 `float64`，包括 `18446744073709551615`。
- `Marshal`：Foundation 风格的**整数限定候选编码**，双空格缩进、排序 key、` : ` 分隔、不转义斜杠、无末尾换行。只接受标准整数文本；小数、指数和负零明确报错，不做有损数字规范化。
- `ValidateCanonical`：仅将输入和本包候选输出逐字节比较。调用者仍须校验根节点、完整 schema、版本、字段取值及引用。
- `Shape`：实现上游 UTF-8 字节长度前缀结构摘要。array 元素 shape 作为保留重复次数的 multiset 排序，因此数组排列可变化，但新增字段、元素数量及字段类型变化不会被忽略。它不是密码学摘要，也不验证语义。

默认限制为 16 MiB 输入、128 层（根节点为第一层）、1,000,000 节点（object key 也计数）。零值使用默认限制；负数无效；可配置深度最多 1024。`Marshal` 和 `Shape` 对人工构造的值使用默认限制，并可终止循环引用。调用者不得并发修改传入的 map 或 slice。

## 证据与未完成边界

依据只读参考代码 `Sources/NoonmarkStorage/NoonmarkDataPackage.swift`、`ExactDateJSONCoding.swift` 及 `Tests/NoonmarkStorageTests/DataPackageTests.swift`。测试使用人工编写的 golden 和合成对抗输入，**没有 Swift/Foundation 实际 oracle 结果**，不能据此宣称 Swift 字节互操作通过。

Unicode key 排序、空容器格式、控制字符转义等仍需真实 Foundation oracle 对账。数字在此候选中限定为整数；未来如 wire schema 含合法小数，必须先取得精确 oracle 再扩展，不能偷偷退回 `float64`。解码后 Unicode key 按原始 Unicode scalar 序列比较，未实现 Swift String 的 canonical-equivalence 语义，因此 Unicode 规范等价但不同序列的 key 也属于待核实边界。

测试覆盖各层重复 key、转义等价 key、Unicode surrogate、无效 UTF-8、资源限制、UInt64 极值、候选编码稳定性、非 canonical 字节变体、shape multiset 和循环引用；附有 fuzz round-trip 测试。
