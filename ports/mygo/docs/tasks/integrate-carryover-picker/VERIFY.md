# 日期组件与 Selected 验证记录

task-id: integrate-carryover-picker

## 本批范围

从正式 14a956c 受控导入纯日期组件、原字节独立交互／像素测试及六 patch 的固定 MyGo 依赖。不含 Day 菜单、业务 mutation、异步输入恢复或 pending variant。组件仅产生含 window/session/source/presentation 身份的 intent，不能写入资料。

中文视觉来自 Swift 43e6eef 的 DatePickerSheet 与设计常量：340 宽、18 padding、304 内容、42 格、40×30 点击区、24 圆形选中、6 hover 圆角、120ms EaseOut 与减少动态效果。真实来源 sheet 截图与 macOS 字体／系统外框尚缺，不能声称像素全等。Go 日期 1–9999 表示边界明确保留，不宣称为 Swift 业务上限。

## 精确来源及透明变更

- 原组件源码 da013fbc01aae33ad3cd66c72a429b00b3fb24f52919b56874ed869dce63bacd，正式保持逐字相同。
- 原作者测试 bd4d6ee993839668ab1b8811f61ca6e69f526f5f05cd178405a4dba0a4d5c9d9，唯一语义变更是将 KnownAccessibilityMapping 旧缺陷诊断明确更名并要求 RoleButton、Selected=true、Checked=false。其旧文件和精确基线原红外置保留，没有删测试来掩盖问题。
- 原独立 reviewer 测试 93a07af9caf5ee362878f0ac26abc66251a3492abc31c3fd0de745a1b1e7283e，逐字不改并成为默认门禁。
- patch 0005 ab7aa8f5be93e4260eddff35728c86792a95d8a16f0b11dd56a396c10a09966a；0006 67fa246685ed116bafde0f0fd56ddacd4cb42da0854b0cb9221374b651645c38；manifest e80ed46c22a2346a011b7755d39a42df000c3e6dbdf647079f669f77a7f6696f。原四 patch、官方 archive 与许可证身份保持不变。
- gates.tsv 只追加本案例。案例补充首次日期、原红实际路径、三处候选通知缺口与回滚说明；不把候选审查问题误称正式生产事故。

## 独立前置验证

原严格 Selected 症状在旧四 patch 为红、最终六 patch 为绿；像素、键盘、动画始终保留。最终220项框架扩展 race、app gates、vet、51项离线依赖／篡改测试、六目标编译与真实默认 check-mygo-native 子入口通过。默认 scripts/check 已通过 Test(Noonmark|...) 匹配新框架回归，不重复添加准备或执行步骤。扩展220项不是默认入口每次全部执行的数量。

24次两示例 before/after release 构建及独立尺寸核对完成。counter-native / hello 增量分别为：Linux amd64 0 / +4096 bytes，Linux arm64 0 / 0，Windows amd64 +2048 / +2048，Windows arm64 +2048 / +2560，Darwin 两架构 +16 / 0。七项增长的包依赖集合均不变，dumpdep/nm 和 ELF/PE 段分析解释选择通知代码、元资料及对齐变化。这是跨编译文件尺寸，不是实机或内存验收。

专项作者的设计与框架指导读取曾晚于首版实现，已有真实记录；不倒填为事前遵循。后续设计、独立审查及正式整合均重新核对相关约束。

## 正式验证

全部源文件、作者及独立测试先整合并冻结后执行正式门禁，代码不在执行中变更。外置证据目录为 noonmark-picker-selected-integration-20261106，integrated-source.json 记录每个导入源的原哈希与正式哈希。

- focused.log：组件、原独立交互／语义／像素、通用按钮及 registry 的 fresh race 通过，11.652 秒。
- check.log：完整 scripts/check 退出0。主模块 fresh race 203.565 秒；未改动的领域与 Service 等模块为 Go 已验证缓存，明确不写成全部 fresh。所有模块 vet、固定 Unicode16 验证、51项依赖测试、MyGo UI native 门禁（2.616秒）及 host popup（1.057秒）通过。
- cross.log：六目标 CGO_ENABLED=0 编译全部通过；只是编译，不是该 OS 的运行验收。
- cli.log：实际 Linux amd64 可执行入口创建／重开隔离 development 资料，渲染日待办／任务池，拒绝重复初始化及隐式无 scope 运行，通过。没有打开真实桌面窗口。
- selected.log：正式专项入口执行框架 Selected 回归及真实组件症状，fresh race 分别1.059秒／1.354秒通过。
- release-unit.log：17项发布流程单元测试通过；这些是模拟/本地约束，不是发布或归档重建。
- 原始 staged git diff --check 对新 patch 数据报 whitespace（退出2）。逐项解析证明所有警告都是 unified diff 必需的上下文空格前缀，不是新增 Go 源码；不能修改已校验 patch 来消除数据格式。非 patch 文件检查退出0，准备后的真实 Go 源码仍由依赖 hash／gofmt／race／vet 门禁验证。原报告保留，不把 raw check 写成通过。

正式文档在测试后补充结果，GO-MYGO-SELECTED-001 会在实现 commit 确定后回填状态，再跑 registry。

## 未完成及安全边界

真实 macOS/Windows/Linux GUI、系统读屏通知顺序、系统 IME、Swift oracle、Swift sheet 实机截图尚未验收。已知框架 TestDiffFrameAllocs 基线失败不在本专项范围，不声称整个上游框架全绿。日期业务入口、完整 Unfinished、成功 toast 与 undo 不在本批。

资料仅通过现有 development/e2e 隔离入口及合成领域命令创建。未接触生产、iCloud、Keychain、真实 AI。GitHub 写入因未决批准暂停，没有新 hosted CI、tag 或 Release。新依赖完整源码包和三轮离线重建需要针对最终稳定源码另跑，a91 的历史打包结果不能覆盖本批。


## 实现提交与闭环

实现已提交为 0fcf95ecc4a134163d7df21215578daa555be89a。随后仅回填案例状态、门禁说明与本段文档，再执行 registry。没有修改已验证的组件、依赖 patch 或独立断言。全量本地 Git bundle 另行保存并校验；远端待批限制保持不变。
