# 对齐普通任务导航计数

- task-id：align-navigation-counts
- 日期：2026-11-05（UTC）
- 级别：A，独立候选；正式目录、Swift 参考、core、Service、codec 与原 MyGo 只读。

## 需求与现状

真实 MyGo 侧栏当前按 rowsFor 长度显示徽标，连 0 也显示。Swift 43e6eef 的 NoonmarkStore.swift:864–885 按领域含义投影，而 WorkspaceShellViews.swift:163 只显示正数，compact 不显示徽标。旧实现由 4cbd56883b8a6065c82902923b66ecc0741a2cef 引入。已有加权 progress 与 title/subtask 草稿机制必须保留。

来源逐项核对：
- day：today 的 getDayTodo 中 pending。getDayTodo 仅抑制旧 deferred，不抑制 pending，因此直接按 today pending 统计等价。不得使用当前浏览日期。
- pool：普通 active chain、有 current definition、没有 pending，并且无 trace 或 latest trace 为 returnedToPool/cancelledDraft。latest 顺序为 contentUpdatedAt、createdAt、UUID 字符串降序。
- upcoming：futurePlans/visibleFuturePlans；当前 Service 只接通普通任务，按有 definition 的 pending 且 date > today 计数。recurrence 窗口不在本批。
- unfinished：普通 active/abandoned chain、有 current definition、至少一个 unfinished/abandoned trace、完全没有 completed trace；按 chain 去重。
- completed：普通 chain 上有 completed trace，或任意 formsDayHistory trace 上有 completed 子任务；都须有 trace definition，按 chain 去重。parent 仍 pending 也可以计数；cancelledDraft 子任务不算。
- 来源：NoonmarkEngine.swift:1574–1630、1720–1803、1926–1949、3782–3798、3845–3920。普通子集不等于产品完整能力；既有 completed/unfinished 页还是 trace 列表，本批不扩展为完整 hierarchy 页。

## 设计（实施前）

复用已验证的 runtimeProgressCache 确认图发布边界：新增只读 navigation 投影 sibling，与 graph/progress 一起发布。它只保留轻量计数和 pending 日期索引，不 NewTaskKernel、不额外 clone。建立 O(chains+definitions+traces+subtasks + pending dates log n)，每帧按 today 查 day、二分求 future，其他常数查询。投影没有自己的可变事实。共享 titleHub 多窗口使用同一缓存；重复图复用对象；拒绝图不呈现过时徽标。today 可变而 graph 不变时查询仍正确。

像素规格保持原样：expanded 侧栏计数 11pt、400、text3、tnum，沿既有右侧位置，不加新徽章；0、未知页和 compact 均不创建计数字形。保留 nav label 的可访问文本，不以测试标签替换用户文字。

## 验证

先新增真实 Service 命令＋MyGo native renderer 症状测试，在旧代码复现红并保存截图；再改投影与接线转绿。覆盖空库、今日完成/撤销、完成子任务、回池/重排、未来取消、自然日变化、重开、失败写入、多窗口 FIFO、浏览日期与 compact、重复帧复用。补纯投影表驱动边界，明确合成资料而非产品 UI 能力。运行 scripts/check、六目标编译、Linux CLI。所有证据放外置 build root。原生 headless 不冒称 OS 实机或 Swift 像素 oracle；当前无 frontend-verify skill、无容器部署入口，以真实 MyGo renderer/Service 和完整门禁补证。禁止 production/iCloud/Keychain/AI/GitHub 写入。

## 候选验证结果（2026-11-05 UTC）

- 旧实现真实三红：navigation-red.log；首轮三绿：navigation-first-green.log。
- 扩展专项 race 通过（33.178s），随后添加构造边界与日期单独发布两项门禁；最终冻结五个 Go 文件完整 scripts/check exit 0，主包 race 186.086s、taskapp 29.818s。完整日志 navigation-check.log 覆盖所有模块 race/vet、Unicode16 离线生成验证、依赖及 MyGo native 边界；不是仅专项通过。
- 六个目标全部编译成功：Linux/Windows/macOS × amd64/arm64。navigation-cross-build.log 为 exit 0。跨编译不是非 Linux 实机运行。
- Linux 二进制 -list 成功；实际二进制创建全新隔离开发 session、2x render、重开同一 session、2x render 全部成功，日志 navigation-cli-session-configured.log。首次额外 CLI 验证漏设 XDG 缓存产生非致命 Fontconfig 告警，原日志保留；显式外置缓存后以新 session 重跑无告警，未改业务代码。
- 实际查看 red-empty 与全部五张 green 2x PNG：空库无 0、完成今日仅 completed=1、完成子项且父 pending 时 day=1/completed=1、45 项列表滚动后的固定侧栏仍为 45、compact 无数字。截图输出位于外置 evidence/navigation-counts；仍不是 OS/Swift 实机或系统读屏器验收。
- 边界覆盖：17 项谓词表、8 种 trace status 的已完成子项、输入顺序倒转、latest 三层排序、历史 definition；真实发布覆盖回池重排、未来取消、浏览日期不改变今日徽标、自然日、纯日期发布、重开、四个失败点和 after-persist 重开差异、双窗 FIFO、无变化/重复帧复用及零查询分配。现有全部 IME、标题/子项草稿、progress 门禁随全量检查通过。
- 交付仅七个 owned 文件及 gates.tsv 单行增量。候选中原有 GO-UI-PROGRESS-001.md 与正式树不同，属于前轮遗留，不在本批，禁止整树复制。Go 哈希、完整 owned 哈希、patch 与 registry 增量放外置 review。
- GO-UI-NAVIGATION-001 保持处理中；独立复审和正式 exact commit 还未完成，不能提前关闭。提交正文须引用 Failure-Case: GO-UI-NAVIGATION-001，Task-Id: align-navigation-counts。

## 正式副本整合与独立复审

独立复审逐项检查Swift来源、五份Go冻结hash及实际原版／MyGo截图；完整scripts/check退出0（root race161.930秒），新增独立测试覆盖100轮集合排列、六种child状态、同链去重、日期边界、深层指针隔离、未知／孤儿图拒绝和恢复相同确认图。独立测试SHA256为8d7b1686552dada0c57e8ad1f69bee01c42d30ec1c38cf18ab57bf395a1e1017，已纳入永久测试。

正式副本8d15eac基线选择性整合七份作者文件、一行gate及独立测试，没有覆盖旧progress故障案例。2026-11-05正式scripts/check退出0（root race167.197秒），四模块race／vet、Unicode16全量离线门禁、51项dependency及native UI/popup通过。六目标cross-build退出0；Linux实际二进制创建全新隔离开发session、day渲染、reopen及pool渲染、拒绝重用目录和未声明身份均通过。证据为外置review/integrated-check.log、integrated-cross.log、integrated-cli.log。

本地门禁不代表新GitHub CI或Swift／OS实机。Completed／Unfinished页面层级投影仍待后续接通；仅修复导航领域计数和零值展示，不能把徽标修复描述为整个页面已完整复刻。待批远端写入未重试，未发行。
