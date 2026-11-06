# 保存中展示验证记录

task-id: clarify-carryover-pending

以下保留 R7 后独立候选的开发记录。正式组合验收与修复提交见文末；各阶段证据不互相替代。

## 来源与差异

来源：R7 35 文件清单 SHA-256 7576deb264386cc41de6e7a8122708a374267ff006eee895ccf2ad56464d7324，复制前全部核对。只在独立源码 `noonmark-go-carryover-pending-copy/ports/mygo` 实施；本批未写正式 `noonmark-go` 或 `noonmark-go-carryover-binding`，不追改整合阶段随后完成的正式基线。

生产代码只改 `runtime_carryover_picker.go` 的 pending 文案、Description 与显示颜色分支。`runtime_carryover_actions.go` SHA-256 de26415a56c6e1e6a1b9313e162f57fa959379bac32c2e590ebbd55e672e1e54 前后相同；writer、Service、coordinator、native 生命周期、MyGo dependency manifest 与补丁均未修改。

既有 `runtime_carryover_pending_test.go` 唯一行为预期差异逐项为：

1. disabled 确认查询的 Label 从「确认延期／确认延续」变为「正在保存延期／正在保存延续」。
2. enabled 关闭查询的 Label 从「取消日期选择」变为「关闭日期对话框」。
3. 真实点击同一关闭节点使用新 Label。相关注释从 Cancel 改 Close。

全部旧禁用、日期/月不变、快捷键隔离、队列只一项、关闭后 writer 继续、导航不被抢断言仍保留；其他既有测试字节不动。未提交组件全部原中文、Label、坐标、字号、颜色、圆角、交互不变。`PENDING_COPY_CHANGE_AUDIT.json` 以原文件精确替换以上三处 Label 和一条注释后，逐字节对照本代旧测试文件。

整合输入为6个可整文件应用文件与单独 `PENDING_COPY_GATE_ADDITION.tsv` 新增行。候选 `gates.tsv` 只纳入7文件完整差异审计清单，不得整文件覆盖正式 R7 闭环状态；整合阶段必须显式追加本案这一行。

## Red → green

新 `runtime_carryover_pending_copy_test.go` 的红绿同字节 SHA-256：4ad12557da1e56095204569b8993560d56cf6270f4a347d8aec112bccee19cf2。

- `pending-copy-red.log`：exit1，三项门禁实际失败；缺保存/关闭语义，旧选择提示仍在，所有日期 Description 缺保存限制，selected 白字与有效日深色像素均为0；真实按钮路径找不到关闭 Label。原 Escape 保存合同仍运行，未误报写入被撤销。
- `pending-copy-green.log`：exit0，新增三项及既有 pending 禁用测试均通过（root 2.394s）。
- 新真实路径共八组合：延期／延续 × 关闭按钮／Escape × 不重开／真实菜单重开新 sheet。准入后重复 Enter 只一 writer；关闭前后磁盘签名不变且 source lease 仍为1；完成后写入一次、lease释放、graph新增一个确认目标；不抢原导航、不关闭新 sheet；关闭 Service 后重开完整 graph 精确相等。

## 像素与可访问验证

实际 MyGo native Tester 2×图：`evidence/green/pending-copy-defer-native-2x.png`、`evidence/green/pending-copy-continue-native-2x.png`，均2400×1536。已逐张实际打开查看：340×395 DIP sheet 完整、标题分别含延期/延续，关闭与保存中未裁切，footer 完整日期可读，蓝圆 selected 白字清晰。无 spinner、新卡片或额外说明块。

另有对应 component 图及原/新 draft 图；本代6张 green 图均已逐张实际打开检查。两种未提交 draft PNG 与改前逐字节相同：

- defer：e0ae850fcc980aa4c948de48d450c2c39735c2e01b6f448508cbc177b83ec432。
- continue：d640b19038ed891d0d4d8ff777b34a2a80fc0599ba5bf1f8a8e03ea5dd3a205d。

`pending-copy-image-comparison.json` 记录 native 截图前后差异仅在原标题/提示、日期格及 footer 按钮三个允许区域；之外像素完全相同。组件测试实测标题110、提示137、保存中36、四位年份双位月日摘要125 DIP，分别小于304／304／52／184可用宽度；原 panel/calendar/summary/两按钮 rect 精确相同。

可访问树验证 RoleHeading、关闭按钮非 disabled 且 Description 仅关展示/继续保存、保存按钮 disabled 且等结果、42日期全部 Disabled=true/Invalid=false、selected trait 仍正确且 Checked=false，Description 说明不能改目标及已提交目标。

## 资源与后续门禁

使用 Go1.27.3、指定共享 GOCACHE、GOPATH=/tmp/noonmark-gopath；GOMAXPROCS=2、-p=1，每次只一个测试进程。新证据/临时 session/prepare dependency 均在全新外置 BUILD，不清缓存、不复制工具链。

定向命令统一 regex：`Carryover|Review|Title|Retirement|ReturnMenu|Navigation|FailureGateRegistry`；packages 为 `.`、`./internal/titlecoordinator`、`./internal/taskapp`。执行 `go test -p=1 -buildvcs=false -count=1 -run ... -v`，再以完全相同范围加 `-race`。结果均 exit0，260个顶层测试、零 skip／fail：

- `pending-copy-focused.log`：root18.659s、titlecoordinator0.007s、taskapp6.068s。
- `pending-copy-focused-race.log`：root161.404s、titlecoordinator1.030s、taskapp30.429s。

范围包含原 Carryover/Reviewer 生命周期强红、Selected语义原红、四阶段错误、retirement、原导航与 return menu 断言；完整 aggregate、六目标编译和发行门禁不在本批重复。另 `pending-copy-vet.log` 与 `pending-copy-gofmt.log` 均为空且 exit0（同三个 package 的 vet、三个变更 Go 文件的 gofmt 检查）。本批末次日志和各 exit 文件为结果来源。

## 真实限制

所有资料仅由领域接口合成 development/e2e。本两张单任务图是局部状态/布局证据，不替代一年中段用户体验验收；年度 fixture 由另一个独立批次完成。Linux headless MyGo native pipeline 不等于 Swift 像素、macOS/Windows实机、系统读屏、系统 IME 或真实 OS。没有操作撤销、磁盘 undo、自动重试；保存中不是成功确认。未触 production、iCloud、Keychain、真实 AI、GitHub，未执行提交／推送／发行。

## 正式组合验收（2026-11-06 UTC）

本节是后续正式副本的实际结果，覆盖本批、保存状态文案、年度R2与普通任务链纯核心的组合；前文候选阶段记录仍保留其原来范围。全部467个源码文件在完整检查、构建及CLI期间哈希/模式保持不变。

- 完整 `scripts/check` 退出0：应用主包race470.810s、taskapp101.273s、taskview1.052s、titlesession1.411s、domain core9.821s；四模块race/vet、固定Unicode16及51项依赖检查、MyGo输入/弹层定向门禁通过。inputdraft、titlecoordinator、textcanon、codec canonical为缓存结果，不称为新跑。
- `scripts/cross-build` 六目标全部退出0：macOS/Windows/Linux的amd64/arm64，只是编译，不是这些系统实机验收。主机构建退出0。
- 实际隔离Linux CLI创建、重开、day/pool渲染、会话复用拒绝及隐式scope拒绝均通过；没有打开真实桌面窗口。
- 证据保存在外置BUILD的 `combined-validation.json` 与 `combined-*.log/.exit`。GitHub写入仍待确认，未推送、没有此组合的新hosted CI；462d6da的旧候选发布复现不能套用到本批。完整产品、Swift实跑、系统IME/读屏及真实三平台仍未验收。

## 提交与案例状态

实现提交 `fbfa97dd810121d3fae268c39cc621882efbaba5`；GO-UI-CARRYOVER-PENDING-001 已在上述本地验收范围内闭环。后续文档闭环没有改变已验证Go源码。GitHub写入仍暂停；不把本地提交当作已推送或新CI成功。
