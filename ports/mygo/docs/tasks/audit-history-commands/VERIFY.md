归档说明：以下为本批设计或隔离验证的阶段记录，其中待实施／待整合措辞指当时状态。原文 SHA-256：`d771396d7717bae1085f6393cb9a7a1d90bcc9d67d315455a341b07c503045da`。归档仅调整协作措辞与任务标识；正式整合结果另见本目录 README。

# 作者交接：未接线 typed history commands

冻结时间：2026-11-06 03:53 UTC。状态：作者隔离验证通过，等待独立复核。不是正式整合、Service/writer/v3或Swift对等验收。

## 交付物

- 源码副本：`/workspace/scratch/586e77bd59a6/noonmark-mygo-work/noonmark-go-history-command-vnext`。
- 补丁：`history-command-vnext.patch`，SHA-256 `9c2c430dddc484990e6625ceaefc6c504671bf0c795de2384786bcd8ebbec601`。
- `FREEZE.json` 记录baseline HEAD/tree、正式status、所有新文件SHA。`new-files.sha256` 可在副本根直接校验。
- `evidence/all-old-files.sha256`：逐文件实读比较486个原tracked文件，副本与正式树全部相同。正式 `03a18cca51766e1d6b501e5e015ce96d0be5a7c7` status为空。
- `evidence/frozen-r1.sha256`：旧r1三个实现和三个测试的原始冻结hash；结束再校验六项全部OK。
- 仅新增5个 `internal/taskapp/history_command_vnext*.go` 文件和1份 `docs/tasks/audit-history-commands/README.md`，共2080行；无删除和修改旧文件。

## 验证结果

所有工具链和环境经候选 `scripts/environment`：Go1.27.3 linux/amd64、外置BUILD/GOWORK/TMPDIR/GOTMPDIR、现有GOCACHE、离线proxy/sumdb、GOMAXPROCS2、GOGC20、GOMEMLIMIT700MiB、-p=1。完整运行时间、退出码和命令见 `evidence/runs.tsv`，各次 `.log` 保留。

- `focused-freeze`：`go test -p=1 ./internal/taskapp -run '^TestHistoryCommand' -count=1 -v`，exit0，包1.770s。
- `taskapp-full`：`go test -p=1 ./internal/taskapp -count=1`，exit0，包25.490s；覆盖全部旧taskapp聚合/reader/Service测试。
- `history-race`：`go test -p=1 -race ./internal/taskapp -run '^(TestHistoryCommand|TestHistoryVNext|TestIndependentHistory)' -count=1`，exit0，包34.150s；新增矩阵及全部旧r1 history测试。
- `taskapp-vet`：`go vet -p=1 ./internal/taskapp`，exit0。
- patch numstat解析成功；新文件hash校验全OK；旧六文件hash全OK；486原文件未变。

没有运行整项目 `scripts/check`、cross/build/CLI门禁或Swift oracle。这些留待整合阶段在独立复核通过后的精确整合树执行。没有生产资料、真实会话、iCloud、Keychain、liveAI、GitHub操作或正式commit。

## 失败记录及修正

所有退出1日志未覆盖、未删除：

1. `red-source`：先写来源正例与malicious delta测试，API未实现，编译明确缺符号。这是预实施红灯，不是旧r1缺陷。
2. `budget-initial`：测试手算string叶36，完整模型实际34；topaggregate样本因整数截断只到999975节点，未超N。按真实字段及checked成本修正为34叶和真正超过N的两个slice。生产预算不变，未弱化断言。
3. `functional-expanded`：rich fixture对无planned children链错误提供SubtaskIDs；core按数量严格拒绝。另旧vnUpdateEvent使用0-based index，错误传1访问唯一事件越界。修正fixture输入及索引，保留原日志。
4. `functional-expanded2`：tombstone恶意变化fixture仅设置DeletedAt而未清Body，domain先拒绝，无法作为“合法graph的恶意delta”。按core约束清空Body，保留必须先ValidateTaskGraph成功的断言。

之后 `functional-expanded3`、`focused-final`、最终focused/full/race/vet全部成功。没有改旧r1、放宽断言、增加timeout或清cache。生产两个新实现自首次通过source聚焦后没有因测试失败修改。

## 实施要点及边界

- 两个caller graph及IDs的metadata-only gate严格在任何Validate/FromDomain/Clone/content scan/index之前。全34字符串叶、23optional、逻辑共享、checked算术、节点N及rawB边界、接近16MiB合法UTF-8、model manifest和AST顺序门禁已运行。
- exact prior绑定及round-trip → proof解码自有graph重放一次 → 完整after一致 → 内部capture → accepted builder → 完整inspector → prefix → 固定冻结段/数量 → 封闭结果。无caller callbacks、captures、exemptions。
- source-positive P/U/C/A/R/D/X、多日期多链稳定排序、silent lock零capture但changed、X-only no-op、old A保留、owner/target/action欺骗、无关整图变化、旧lock、signedzero/MaxFloat/日期端点、zero结果及alias/concurrency通过。
- 两端reader-valid的旧事件ID/clock/删A负例是prefix层测试；command after参数不含classifications，未声称入口直接接到该恶意字段。
- 小写task rawValue被必要条件reader接纳后无法精确回组，命令拒绝；小写旧classification event/key/traceID则实际通过complete/undo/append并保留旧字节。
- 结果不是写能力，不是Service时钟，不是跨会话提交凭据。caller不得并发修改输入。纯helper的race不替代磁盘/双handle/reopen/Today/UI测试。
- 现有Go Now<chain clock拒绝保持；Swift max差异没有本轮实证或修复。Go固定排序也不声称Swift字节等价。

后续如独立复核发现问题，应另记修订，不覆盖本冻结patch和日志。整合阶段批准并完成独立验证后方可整合。
