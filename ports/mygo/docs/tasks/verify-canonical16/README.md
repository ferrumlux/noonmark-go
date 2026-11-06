# 固定 Unicode16 候选与聚合门禁

日期：2026-11-06（新加坡）。级别 P：名称语义与验证输出的安全边界；本批维持独立候选，不变更运行资料或开放非空 catalog。

## 方案和范围

在 domain-extensions/classification/canonical16 保留固定 Unicode16 的 DisplayName、NFKC_Casefold、NFC 与规范等价实现、官方输入、生成器及 Unicode V3 许可。原 classification engine、v7 reader、Service 和 UI 不接线，旧 adapter 不被隐式替换。公开 canonicalKey 只做一次 display；实体规范器的双层 display 另行覆盖，不把它误用为完整分类工作流。

27 份候选文件 bytes／mode 已与独立审阅 freeze 核对。清单 SHA-256：b13a0d623a9ee4a903695265ce5668bad182a6ac2d42b12df8c037a2b67a0f31。生成表 SHA-256：39c0bba7e56b499ab71457dfe0c5e3ed81d76211ae2f9009c2d82f689bf912dd。官方输入、发布来源和复跑命令见包内 README 与 testdata/ucd/SOURCE.json。

scripts/check 现在额外强制执行候选离线 verify：官方规范、全部合法 scalar、数据与生成器身份、重生成逐字节比较、篡改拒绝、路径保护及六目标纯 Go probe 编译。普通聚合入口明确 RUN_ICU76=0，不把 Ubuntu 默认 ICU 版本误称为 ICU76。新增根 Go 测试实际调用路径保护与离线生成脚本，故障 GO-CLASS-002 纳入统一 gates.tsv。

## 实际验证

正式整合的最终 scripts/check 退出 0：四模块 race／vet；应用主包 85.875s；候选专门无缓存 race 3.075s；51 项依赖门禁；MyGo ui 2.772s／root 1.061s。27 项危险路径在写入前拒绝，两次正常入口只创建不同 0700 run，旧证据不变。

同一源树六目标应用 cross-build 退出 0，Linux development CLI create／reopen／拒绝覆盖／无隐式创建和 day／pool 2400×1536 离屏绘制退出 0。17 项发行脚本单测、17 个 workflow 负例、三份 workflow 的 actionlint 和 git diff --check 通过。这些不是新候选发行包的完整三轮构建，也不是其他 OS 的实机运行。

候选开发和独立审阅曾实际通过 ICU76.1／Unicode16 全部 1,112,064 合法 scalar 及 156,435 跨 scalar 语料差分。安全修复后算法与数据 23 文件未变。正式聚合此次没有重复 ICU 差分，不把沿用证据写成新执行。19,965 行 NormalizationTest、1,093 行 GraphemeBreakTest 是官方规范，不是实际 Swift oracle。

## 风险、回滚和后续验收

Swift capture 仍待真实 macOS CI；Swift Character 使用的 Unicode 版本没有独立公开，ICU 版本不能替代它。完整分类 prepared command／receipt／audit、导入导出及 Swift 实体 decode 差分仍未完成。因此不得将本候选作为迁移器，或放宽现有 fail-closed reader。

若验证失败，停止接线并保留证据；可撤回独立包、两个根测试和 scripts/check 新入口，不改已有用户资料。灰度范围只有合成 development／e2e fixture，没有生产灰度。每个获准的新远端 head 需运行 GitHub CI，随后才决定更广范围的兼容验收。

当前远端写入待确认，本批本地通过不等于已备份至 GitHub；没有 tag、正式发布或生产数据访问。
