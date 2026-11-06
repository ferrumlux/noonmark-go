# 固定 Unicode16 分类名称候选

这个独立、纯 Go、无 CGO 的包实现固定 Unicode 16.0.0 名称算法。它没有安装到分类 engine、codec 或 runtime，也没有改变非空 catalog 的封闭门禁。当前证明范围是官方 Unicode16 规范和系统 ICU76／Unicode16 差分，**不是 Swift oracle，也不是完整分类业务对等**。

## API 与精确调用次数

- `DisplayName(s) (string, error)`：以 Unicode16 extended grapheme cluster 分段，只根据每簇首 scalar 的 White_Space 判定 separator，按源 `split/join` 折叠为 ASCII SPACE；其余簇原样保留。
- `CanonicalKey(s) (string, error)`：对应上游公开 `canonicalKey`，恰好一次 `DisplayName(s)`，然后 NFKC_CF 与完整 NFC。
- `NFKCCasefold(s) (string, error)`：纯 NFKC_CF／NFC，不调用 display、不 trim。
- `NFC(s) (string, error)`：真正的 Unicode16 NFC，完整 canonical decomposition、稳定 CCC 重排、完整组合及 Hangul；没有 stream-safe 限制、自动 CGJ 或非 starter 数量截断。
- `Equivalent(a, b) (bool, error)`：只比较 Unicode16 canonical equivalence，不做大小写、兼容、空白折叠。
- `Canonicalizer{}.Version()/Canonicalize(s)`：method set 满足父包 `NameCanonicalizer`。它只模拟直接 `TaskCategory`／`TaskLabel` initializer 的两层调用：`display = DisplayName(s)`，`key = CanonicalKey(display)`，再执行非空 display/key 约束。原始 Swift initializer 自身不执行此非空拒绝，约束来自上层名称校验。因此这个 adapter **不代表所有上游 workflow 的无缝替代**。

版本固定为 `unicode-16.0.0.0-nfkc-casefold-whitespace-v1`。所有 API 对无效 UTF-8 返回 `ErrInvalidUTF8`；合法 U+FFFD、NUL、noncharacter 和未分配 scalar 都合法。纯转换允许空串；adapter 对空 display／空 key 分别返回 `ErrEmptyDisplay`／`ErrEmptyKey`。名称失败时不返回部分结果。没有私自施加长度限制，资源消耗由调用方按应用边界控制。

### Display 并非幂等

输入 `a\n\u0301b` 的第一次 display 为 `a \u0301b`，第二次为 `a b`：插入 ASCII SPACE 后，combining acute 在下次分段中加入空白簇。因此公开 `CanonicalKey(input)` 与直接实体构造的 `CanonicalKey(DisplayName(input))` 不一定相同。不得删除“多余”调用，也不能擅自增加 display 重算。

上游只读基线为 `43e6eefaae3a04be89cab3e234d470111afd80ef`，`Sources/NoonmarkCore/TaskClassification.swift`：

- `displayName`／`canonicalKey`：16–32。
- `TaskCategory` initializer：262–263，先 display，再 canonicalKey(display)。
- `TaskLabel` initializer：353–354，相同两层调用。
- `normalizedClassificationName`：3076–3085，先 display，再调用 canonicalKey(normalized) 做空 key 检查。
- prepared create、resolve、rename 另有 preprocessing；须由完整分类模型逐条对齐，不能由这个 adapter 推断。decoder、名字等价与历史规则也仍须真实 Swift 验证。

## 数据、算法与许可

`testdata/ucd/` 保存最终发布 Unicode16 的实际输入与规范测试，附完整 Unicode License V3、最终 ReadMe 和固定 `SOURCE.json`。官方包为 https://www.unicode.org/Public/zipped/16.0.0/UCD.zip ，SHA-256：`c86dd81f2b14a43b0cc064aa5f89aa7241386801e35c59c7984e579832634eb2`。九份算法／测试文件与 `unicode-org/unicodetools@b91447019d1b785cd5e82193936e3c9209852e9b` 对应目录逐字节相同；最终 ReadMe 不采用 repo 模板。archive 本身不重复提交，来源和逐文件 digest 已保留。包内 `.gitattributes` 强制 LF，避免 Windows checkout 的自动 CRLF 转换破坏官方字节及生成一致性。

生成器只解析这些文本，不使用 Python `unicodedata`、Go `unicode` 或 x/text 的 Unicode 表，不联网，也不依赖系统 UCD。它校验固定 manifest 本身、每个源文件及许可的 SHA-256，再生成确定性的 Go 表。输出头记录 manifest 和 generator SHA；输出不嵌入时间、绝对路径或 Python 版本，离线重跑逐字节相同。

EGC 包括 GB3–GB13（CRLF/Control、Hangul、Extend/ZWJ、SpacingMark、Prepend、GB9c InCB、GB11 EP、RI 奇偶）；InCB 按官方三列解析。NFC 使用 DerivedNormalizationProps 的 **Full_Composition_Exclusion**，不是只使用显式 CompositionExclusions 子集。全部 nonstarter run 稳定计数排序，不作平方插入排序。NFKC_CF 使用空映射删除 default-ignorable scalar，随后对整个结果 NFC，允许跨原始 scalar 的重新组合。不在 fold 后 trim，U+00A8 的 SPACE+0308 必须保留。

## 必需门禁与离线重现

常规 Go test 会执行完整 19,965 行 NormalizationTest（每行五组 NFC 及 NFD）、1,093 行 GraphemeBreakTest、全部 1,112,064 合法 scalar 的 NFKC_CF 和 NFC/NFD Quick_Check，以及 invalid UTF-8、空 key、blocking、DI 删除后组合、并发及 29/30/31/32/60/61/4096/65536 长 run。默认 Go test 还固定校验 manifest、原始数据、generated 表 SHA 和生成器来源，防止手改表仅凭少量样本碰巧过关。

独立入口 `scripts/verify.sh` 在首次 mkdir、日志及 Go 调用之前先执行只读路径门禁：保护整个 Git repository、候选 aggregate（包括 domain-core／app 兄弟目录）及相邻源码 repositories；build root、GOPATH、GOCACHE 必须是绝对规范路径、彼此不重叠、不包含源码；拒绝双前导斜线、重复斜线、dot／parent 组件及所有不可打印字符（包括尾换行）。路径组件不能含 symlink，现有输出／cache 树只允许目录和 link count 为1的普通文件，拒绝 symlink、hardlink、FIFO／socket／device 等 special file。Go module cache 和临时目录也受控外置。27 个实际入口／完整 repo 布局负例会验证 module 内、app 兄弟目录、repo 内、源码祖先、各类 symlink、重叠 cache 和相对路径均在首次写入前被拒绝。所有拒绝都比对合成 canary 与测试树未变且禁止调用 Go。guard 不输出路径给 shell command substitution；入口直接保留已验证环境变量的原始字节。验证通过后只在外置 base 内独占新建随机 `run-<hex>` 目录，权限0700，日志、生成副本和二进制不复用任何旧run文件；两次正常外置调用回归证明目录不同、旧证据字节不变。

该入口是完整合并门禁，**不能只用父项目当前的 Go test/vet 代替**。它另外执行离线重生成与逐字节 cmp、修改一份外置 UCD 副本的 hash 负例、race、整个 domain-extensions 回归、vet、逆序长 run benchmark 和六目标 CGO=0 编译。普通运行不需要 ICU；设置 `RUN_ICU76=1` 后，缺 ICU76 或 Unicode 不是16均失败，不静默 skip。集成到正式 CI 前，应把该独立入口加入 CI 必跑步骤；本候选不修改正式 CI。

执行前将 `GO` 指向固定 Go1.27.3，显式设置外置 `NOONMARK_MYGO_BUILD_ROOT`、`GOPATH`、`GOCACHE`，然后运行：

```sh
GO=/absolute/path/to/go1.27.3/bin/go \
GOPATH=/external/gopath GOCACHE=/external/gocache \
NOONMARK_MYGO_BUILD_ROOT=/external/canonical16-build \
RUN_ICU76=1 ./classification/canonical16/scripts/verify.sh
```

也可直接离线运行 `python3 classification/canonical16/internal/generate/generate.py --check`；`--output /external/tables.go` 不覆盖候选源。不要对生成表手工 gofmt 后略过生成一致性校验；当前生成输出本身已符合 gofmt。

ICU 差分脚本固定 seed `0x16c0ffee`，对所有合法 scalar 和 156,435 条跨 scalar 语料比较 NFC、NFKC_CF、ICU root EGC＋首 scalar White_Space、公开 key、直接 initializer 的 display/key 和名称错误。语料包含完整规范测试、DI mutation、兼容映射邻接、随机／偏置 scalar 和长 run。脚本、输入、输出及 probe SHA 都写到本次独占run下的 `icu76/RESULT.json`；入口在 stdout 打印本次run位置。跨平台编译只证明编译，不是 macOS／Windows 实机执行。

## 风险与后续

Swift 的 `algorithmVersion` 只编码 ICU Unicode 版本，不编码 Swift Character segmentation／String equality 的实际版本。仍须在各目标 Swift runtime 实测这些行为并保留精确来源；不能把 ICU 或规范测试改称 Swift oracle，不能自动迁移旧 canonicalKeyVersion 或伪填16。没有修改旧名字所有权、历史、审计模型、落盘或 runtime 门禁。完整分类业务接线应在这些条件全部满足后另行进行。
