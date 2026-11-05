# Unicode 标题比较修复验收

隔离基线：`c619c7824f1cb16c4bf25ebc57e14e86a634e659`。先在独立候选验证，再逐文件哈希核对后合入正式副本；Swift 与 MyGo 原仓库未修改。案例 [GO-RUNTIME-011](../../engineering/failure-cases/GO-RUNTIME-011.md) 记录引入提交、原始反例、证明、永久门禁和风险。

## 源码交付

- 新共用包 `domain-core/textcanon`：`Equivalent(a,b string) bool`，非法UTF-8 fail-closed，不重写输入字节。
- 三个比较点 `core/title.go`、`internal/inputdraft/gate.go`、`internal/titlesession/session.go` 替换stream-safe整串NFC。
- 三个独立domain模块最低Go同步1.27.3，保持Unicode17，不改任何依赖版本或sum。
- 五层 `*_streamsafe_test.go` 与共用包测试形成永久门禁。本目录 `gates.tsv` 单行已追加正式总表，注册测试同时纳入 textcanon；没有覆盖并行工作。

## 已验证

- 修改三个调用前，core、Service、gate、session、真实MyGo Tester UI的原始CGJ碰撞均捕获红色；同一用例修复后-race转绿。含保存期间新输入、错误确认、原字节/磁盘revision/重开、错owner与无效UTF-8拒绝。
- 追加invalidUTF8核心入口8个红case在显式拒绝后转绿；Service12分支原本就拒绝且无写入，永久保留守卫。`red-invalid-core.txt`、`red-invalid-service.txt`及对应green日志记录范围。
- 共用比较器测试穷举1,112,064个合法scalar，锁定Unicode17，单scalar最长nonstarter run为3；测试29/30/31/32/60/61/4096/65536边界、逆序/同CCC、真实CGJ、Hangul、emoji和组合字符。
- 独立Linux ICU76以固定种子对9001共同字符输入对执行NFD对账，全部一致。外置脚本和语料可复跑；不声称Swift oracle或完整跨Unicode版本对等。
- 65,536+65,536个combining mark的反序比较单次约23ms、8.2MB，复杂度由固定256桶算法保证O(n)，单次计时不是性能保证。
- 独立只读审查未发现可复现缺陷；另独立运行textcanon/core/inputdraft/titlesession测试通过。
- 六目标交叉编译通过：linux/windows/darwin × amd64/arm64。Linux CLI身份、新建、重开、拒绝覆盖/隐式启动及day/pool离屏渲染通过。

## 外置证据

build root `/tmp/noonmark-unicode-build`：

- `red-{domain,internal,ui}.txt`：原始症状红色。
- `green-{core,routes}.txt`：同样症状与补充守卫-race绿色。
- `evidence/comparator.log`：scalar穷举与长串benchmark。
- `evidence/icu-compare.log`：独立ICU对账；`icu_compare.py`、`icu_compare.go`、`icu_cases.json`为可复跑材料。
- `evidence/check-final-utf8.log`：最终整仓gofmt/race/vet/依赖与native门禁。
- `evidence/cross-final-utf8.log`、`evidence/entrypoints-final-utf8.log`：编译与Linux CLI。

候选及正式完整 `scripts/check` 均已 exit0；原始红日志保留，不以focused通过替代完整验收。构建环境使用显式Go1.27.3、GOTOOLCHAIN=local、官方GOPATH和外置GOCACHE；生成资料均为development/e2e合成资料。生产/iCloud/Keychain/AI未启用。

仍未验证：Apple/Swift动态oracle、真实操作系统IME与macOS/Windows实机。没有数据迁移，不将离屏Tester或六编译冒称完整平台对等。

## 正式整合

实现提交 [ab46ea6580fffd9cabb0e97b2a84b6c38c5a14f5](https://github.com/ferrumlux/noonmark-go/commit/ab46ea6580fffd9cabb0e97b2a84b6c38c5a14f5)。2026-11-05 19:33–19:39 UTC，正式副本完整 scripts/check、六目标cross-build和Linux development CLI全部exit0。应用main race56.101s、taskapp16.035s、core1.278s、textcanon1.556s；native ui2.632s、root1.051s。51项依赖准备测试通过，day/pool离屏帧各2400×1536。16个冻结候选文件哈希全部一致，另仅追加统一门禁登记与registry目录。

正式记录为外置 `noonmark-unicode-integrated-{check,cross,entry}.log`。新精确main b5b9345 的 hosted CI 已实际完成，run 42096453574 于19:56 UTC全部成功；job和证据ZIP已核对，详见GITHUB-CI.md。没有借用 c619c78 的绿色结果。
