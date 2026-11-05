# GO-CLASS-001：规范等价显示名不应制造分类重命名

日期：2026-11-06（新加坡）。引入提交 `8b273d93d06b9938ee422e50f99632ab4e3e3fbc` 的第二批分类低层 Engine 使用byte equality判断rename no-op。该模块尚未接通运行Service/非空catalog；此例不是生产资料损坏报告。

上游 `Sources/NoonmarkCore/TaskClassification.swift:3487,3581` 分别使用Swift String equality比较分类/标签显示名。外置探针在旧正式engine证实 É→E+COMBINING ACUTE错误Changed=true、revision1→2、两条nameVersion。新增原反例在修改实现前实际红：category/label各覆盖accent、Hangul和跨CCC重排，共6分支。仅fold-key相同的显示名对照原本为绿，不能把它们一起抹为no-op。

根本修复仅将显示名比较换成已统一审查的 `domain/textcanon.Equivalent`。不使用NFKC_Casefold key相等，不用会自动插CGJ的整串NFC/NFD。规范等价保持实体全部原字节、名称历史、更新时间和revision；该Go低层模块仍记录自己的no-op receipt用于幂等重放，不声称这等于Swift完整receipt wire。ABC→abc、全角→半角、Straße→STRASSE、真实CGJ不同仍为合法显示改名。

旧错误engine产生的合成snapshot已由实际执行导出到 `classification/testdata/legacy-equivalent-rename.json`，没有手工编造历史。修复后的严格self-replay拒绝该错误advancing record，不修改输入或文件，不能为迎合新算法静默重写旧记录。本模块不是runtime v2数据格式，也不是Swift包；非空catalog gate仍拒绝，禁止打开/迁移生产资料。若持有此低层旧实验snapshot，应保留原件，不把源码回滚/升级当迁移。

永久门禁：`TestRenameCanonicalDisplayNoopPreservesFacts`、`TestRenameSameFoldKeyDistinctDisplayMustChange`、`TestRenameLegacyEquivalentRecordRejectedWithoutRewrite`，统一由scripts/check运行。casefold adapter仍只有精确synthetic fixture，不因修复此相等条件宣称实现NFKC_CF、grapheme空白、完整审计或Swift互通。固定Go1.27.3/xtext0.43.0下textcanon为Unicode17，跨版本完全对等仍需真实Swift oracle。
