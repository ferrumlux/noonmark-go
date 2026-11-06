> 阶段归档：独立副本审查记录；正式整合及当前状态以本目录 README 为准。

# Title/manual 历史 adapters 独立实现审查

## 范围与独立性

正式基线 `2a808a3c4fe788c69d8f47cf21e1d686860a19ff`，独立 candidate 从精确 git archive 建立，仅复制作者冻结的三个 production 文件。没有读取或复制作者本轮新增/修改测试。使用基线已有 helpers，另自行编写 `history_edit_independent_test.go`。只改独立 candidate；正式、作者与原上游树未改。没有网络、production、iCloud、Keychain、真实 AI、缓存删除、root UI 或六目标运行。

设计三文件 SHA 与设计独审 SHA 均与委派一致。读过 ports/mygo/AGENTS、上游 AGENTS 与设计/矩阵/独审。源 manifest 记录文件级对照；74 个旧 production 文件只有 history_command_vnext.go 变化，其11个原函数逐体对照只有指定 valid() 注册例外变化。其它旧生产文件字节不变。

## 静态判断

- 新入口严格顺序为 seal、before budget、after budget、input metadata budget、立即 bind。metadata 仅长度及固定算术，不读取字符串内容或 clone。
- owned graph 来自 sealed prior 解码；不使用 caller before 代替 owned。caller before 经完整 canonical 往返绑定，after 经完整 replay 匹配。finish(nil) 保留完整 inspector、prefix、capture count 与 supplement fence。
- fixed-field Clone+reflect fence 没有开放通用 caller allowlist。title version 只开放当前定义 supersession 元数据、一个追加定义、pending reference/clock 与 chain max；manual 仅选定 trace manual/content 两字段。
- owned decode 将 wire 数组统一为空 slice；kernel clone 保留该表达，避免 caller nil/empty 区别误拒。新定义 expected 可引用旧字段供只读比较，但不回写，也不向结果暴露 pointer/slice。
- core title 的未消费 ID/Now、canonical no-op、版本与reuse差异、signed-zero严格数值max均保持。manual沿用 core owner ErrInvalidInput/nonUUID36 trace ErrNotFound，且guard先于no-op；无全局正数clock或Service严格后继规则注入。
- 未发现需改 production 的实现缺陷。两处旧未知kind测试与合法注册扩展冲突，均有真实红证据并仅修未知哨兵，不删除断言。

## 独立执行覆盖

首版10个顶层测试，参数循环覆盖：

- 实际 core + 原 typed lifecycle audit 组成0/1/2事件 prior，Rename版本化/Input复用及后续逐字复用；完整旧classification相等。
- unused ID非法UTF8/NUL，NaN/正负Inf/早时钟/MaxFloat no-op；manual nil→0与已有0 no-op。
- manual负极值、0、50、100、正极值clamp，100不完成且chain不变；owner及36字节非UUID错误层级。
- independent title/manual fence负例：历史正文、description、状态、manual字段、定义顺序；caller before/after及returned AST不污染封闭结果。
- input raw N/N+1、before/after/input预算与seal先后，失败零proof。
- canonical-equivalent保留原bytes、±0双向tie、negative/equal clock参数（明确不是Service生成证据）。
- typed settlement→实际core continuation的历史floor60，title Input fork保留旧trace正文；explicit过去trace不能借同链另一个P。
- 同日同seq R/P peer不计floor；未来R实际child阻断当前P；pool planned body及Rename version exact copy。

fixture 调整记录：初版错误假设 return/re-schedule 会递增seq，实际核心保持0，现测试其正确的同组peer排除；初版试图同链再Schedule第二P，被核心inTaskPool guard拒绝，改为真实未来R child后移除pool规划、再当前日排程。此时间请求顺序是明确pure profile边界，不声称Service可达。原失败日志完整保留，未放宽validator。

## 尚未独立穷举的边界

同日较小continuationSeq混合group没有独立fixture；同日同seq排除和更早日floor已有真实覆盖。各字段shape/bytes全部精确N/N+1、shared string累计所有组合、planned-only manual放行、全部状态guard组合、long-combining文本、restored-current successor witness等没有在新增独立文件穷举。新入口AST gate没有新增独立可执行解析测试，本次做源码逐项审查；原完整套件没有因本轮扩大其AST覆盖。须与作者测试和整合任务最终union门禁合并判断，不把这些未测项写为独立实测。

没有Service/store/UI接线、真实writer重开、Swift oracle、跨平台、完整产品验收结论。

## 证据

所有日志含精确命令、UTC start/end、exit、Go1.27.3；使用显式外置BUILD、scripts/environment、现有GOPATH/GOCACHE、离线proxy/sumdb、GOMAXPROCS2/GOGC20/GOMEMLIMIT700MiB/-p=1，无timeout/skip调整。

- registry-red.log：旧末kind+1真实红。
- focused-expanded.log：上述两fixture假设真实红。
- full-taskapp.log：旧6..9未知kind范围真实红（31.410s）。
- registry-tests.patch：两处最小哨兵调整，保留全部拒绝断言。
- independent-tests.patch / history_edit_independent_test.go：完整自写artifact。
- FROZEN-PRODUCTION.tsv / FUNCTION-FREEZE.tsv：旧production与函数冻结。
- run.sh：实际运行器与环境。

## 最终结论与运行版本

**Accepted：在这三份冻结production实现与本报告明确测试边界内，没有发现阻断实现缺陷。不是全产品或Service/store/UI验收。**

首版10个测试的未过滤完整taskapp：30.268s，06:06:10–06:06:46，exit0；完整race：179.145s，06:06:46–06:09:54，exit0；vet：06:09:55，exit0。日志分别 full-taskapp-final.log、race.log、vet.log。该源码版本是 FULL-RACE-SOURCE-SHA256.tsv。

在这三项全部终态后，按整合任务要求追加2个独立测试，总计12个顶层测试：

1. 从真实成功封闭结果克隆proof，遍历uint8全部0..255，只允许当前已注册1..8；包含紧邻9、0与高值，不用所有非零合法假设。
2. 实际core与typed生命周期adapter生成两条链各两次A/P事件；对canonical事件字典key、event id、traceID使用小写表示，并交换两条stream及latest的dictionary pair排列。经完整inspector重新取得合法prior，再经新title trace-route零capture入口，断言整个classification表达逐字相同。没有造seal或放宽profile。

最终全部12个独立测试focused：0.357s，06:10:04–06:10:10，exit0，见 focused-supplement-final.log。FINAL-SOURCE-SHA256.tsv为此最终版，只有新增独立测试文件与full/race版本不同，production完全相同。整合任务明确接受此前full/race冻结版加追加focused证据，最终union全量由整合任务执行；不声称追加两测试已在本独立树跑过race。

最终production SHA：
- history_command_vnext.go：815c1bd29559f0fd8a393247c57c192c88642fd46f2ec4093398f37a62c34b21
- history_edit_command_vnext.go：d46a60bdd96577d3f4c6e3dbed9fb62c0e5fc02261fae0ef4cbf1e2d2b35a257
- history_edit_command_vnext_budget.go：8e09e8d0d2c32813681e7be45d9ac33cecda74112a6a6567ed1b904eb9cdadd1
