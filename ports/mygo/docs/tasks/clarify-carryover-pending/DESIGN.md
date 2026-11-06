# 日期保存中展示语义

task-id: clarify-carryover-pending

级别：A（窄 UI 改动，含真实异步交互、可访问语义和落盘回读验证）。

## 需求与事实

本批只补 Go + MyGo 异步 port 新增的 pending 阶段，非 Swift 原版错误。R7 已冻结数据安全合同：确认经原生屏障且命令准入后，目标日期不能更改；关闭展示不撤销已经准入的 writer，也不释放来源 guard；旧完成回调不能关闭新 sheet 或抢导航。本批不改这些行为，不新增事务取消、磁盘 undo、自动重试或业务状态。

现有 pending 已禁用日期、月份、今天、确认及键盘修改，却仍显示「请选择…」「取消」「延期／延续」，没有保存状态；所有日期一律采用不可选择浅色，选中目标辨识不足。此差异由源码与新增 red Tester 症状门禁核实。未提交状态仍须逐字、逐布局保留。

输入为 R7 35 文件 manifest，SHA-256 7576deb264386cc41de6e7a8122708a374267ff006eee895ccf2ad56464d7324，已全部核对；来源与正式目录只读。本批使用全新独立源码及外置 BUILD，不复制工具链、Git 或缓存。

## 像素级规格与文案

仅 pending 改动；保留白色 340×395 DIP sheet、18 padding／304 内容宽、radius12、原阴影；标题 16/600、提示 11/400，原 y=0/24；月标题、42 格、40×30 hit、24 圆及 footer y=318/331/336 全不动。按钮 radius7／高28，日期 hover radius6；字体和 sourceVisual 色 token 不新增。

- 标题：「正在保存延期…」或「正在保存延续…」。原位置显示同字号，不新增 spinner／卡片／说明块。
- 原提示行：「关闭不会撤销已提交的操作。」（11 号字体测量须 ≤304 DIP）。这是独立且必要的状态语义，替代选择指引而非叠加。
- 原取消按钮仍 52 DIP，文字「关闭」，accessibility Label「关闭日期对话框」，Description「仅关闭日期对话框，已提交的操作会继续保存。」。
- 原确认按钮仍 52 DIP、disabled，文字「保存中」，Label「正在保存延期」或「正在保存延续」。Description 明确等待结果，不把 disabled 状态冒充可以取消。
- 日期仍为完整年月日摘要，保留蓝色选中圆；pending 时 selected 白字，原本可选的日期沿用 text1／text3，原本不合法日期保留 line2×0.72。所有日期仍 disabled，不通过外观恢复可操作性；仅 pending 已选及合法日期使用公开 DimDisabled(false) 避免框架重复半透明，禁用 input/accessibility 保持不变。
- 日期 Label 仍使用原「选择日期 YYYY-MM-DD」作为控件身份，Disabled=true 明确不可操作；pending Description 附「保存期间不可更改目标日期」，selected 附「已提交的目标日期」。Panel Description 同时给出保存中及关闭非撤销含义。未提交 Label／Description 与像素不变。

使用原位置替换后无需加高或缩小文字。完整日期在 184 DIP 摘要须实测最长四位年份／双位月份日文案不裁切；必要时报告，不能静默改变未提交布局。

## 实施边界

只改 runtime_carryover_picker.go 的 pending 视图分支。辅助 button 可返回节点以只在 pending 添加 Description；默认分支保持其属性与顺序。runtime_carryover_actions.go、writer、Service、coordinator、native lifecycle 均逐字不动。

新增组件测试先在原实现判红：明确状态文案、关闭语义、所有禁用/Selected trait、目标描述、原坐标/尺寸、可读 selected 像素及未提交基线。真实 Tester 通过原生菜单确认进入 pending，分别点击关闭及 Escape，再确认 writer 继续落盘、不重复、关闭不抢导航；另从真实菜单重开新 sheet 验证旧 completion 不误关。回读同次隔离持久化 session，按完整 graph 精确相等。

既有 pending 测试仅替换三处 label 期望（保存确认、关闭查询、关闭点击）及注释，不删任何禁用、目标冻结、shortcut、队列、落盘、不抢导航断言。所有旧原红字节/断言保留并定向/race 回归。

## 验证、风险与回滚

先跑新 red tests，后 focused 与 race；资源 GOMAXPROCS=2、go -p=1，使用整合阶段指定共享 GOCACHE/GOPATH，每次只一个编译/测试进程。不自行重复全量，聚合门禁由整合阶段后续合并年度 fixture 后统一执行。2×真实 MyGo Tester 截图分别覆盖延期及延续 pending，每张实际查看，断言边界不裁切、日期选中仍可辨；保留相同源代码未提交截图像素基线。

所有 session 为领域接口合成 development/e2e，不碰 production／iCloud／Keychain／真实 AI。Linux headless native pipeline 不代表 Swift 像素、系统读屏、系统 IME 或真实 OS 验收。没有独立 Swift sheet screenshot oracle。保存中是等待确认，不声称已写入成功；错误及不确定完成沿用现有 runtimeError，不添加重试。

回滚仅撤本批 pending 文案与显示分支；不回滚、删除或改写任何已确认数据。无 Git commit、远端写入或发行。故障案例状态保持处理中直到正式门禁与修复 commit 均完成。
