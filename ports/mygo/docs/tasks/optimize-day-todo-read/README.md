# DayTodo 窄读取独立候选

- Task-Id：`optimize-day-todo-read`
- 基线：`3346d052e5a3b7ee536f7b5dd0560d31a7147438`，tree `a43f13f95ddb6d1b80996c965983327c65c2c396`。
- 本候选只供后续独审与真实门禁，不是正式合入。正式与 Snapshot I1（eb924/3346相同领域tree）未改；未吸收分类候选，无 push。
- 全部测试为合成资料。没有 production、iCloud、Keychain、真实网络 AI 或跨设备验收。

## 产品差异与版本绑定

仅 `domain-core/core/kernel.go` 的 DayTodo，以及 `domain-core/core/clone.go` 的 trace clone 抽取变化。一次 RLock 内执行两轮扫描并深拷贝保留行，局部 defer 解锁后才按原比较器排序。共享 `cloneDayTrace` 保留九指针、notes backing array 与每个 DeletedAt，nil/空区别及 Instant 原位模式。

仍是两轮 O(T) 扫描与 O(R log R) 排序，没有缓存、索引、借用或公开 API 变化。NewTaskKernel、Snapshot、mutate、验证、ProjectDay、recurrence与其他产品函数源码未改。

`FROZEN-PRODUCTION-OVERLAY-v1.tsv` 仅两条精确 old→new SHA256。原历史清单、Service/Day detail/Day snapshot 三条 overlay 和旧负例原字节保留；消费链在三阶段之后才应用本候选。任意其他合法新hash、错旧hash、路径遍历、重复/缺失/额外行、无变化及已前进输入均拒绝，成功与失败均不改输入。AST测试检查消费链及锁内两轮/clone、锁外sort形态。父级独立源码审查批准这两个绑定；随后独立审查overlay helper及消费链，并真实重跑相关race门禁（exit0，1.048s），允许本地候选定稿，不等于完整验收。外置回执：day-read-build/parent-source-review.json、day-read-parent-check/freeze-review.json。

`SOURCE-MANIFEST-v1.json` 列出基线83份非测试Go产品源码的新旧hash，仅上述两文件不同。两个模块的历史 evidence/source-manifest.json 不重写，旧证据不冒充新版本证明。

## 行为与所有权验证

独立冻结旧全图clone与DayTodo oracle不调用新Clone/helper。公开合法图覆盖状态、隐藏history、严格较晚遮蔽、等时、跨日、负时间/±0、nil/空notes、非法查询日期和命令结果。手写expected IDs与排列变换提供独立期望。

九指针与所有note字段由纯值helper specimen覆盖，明确不声称该全字段组合是合法领域图。反射字段/类型及嵌套引用形态清单新增字段即失败，独立递归地址检查和实际改写验证输入、Snapshot、两次返回及同行间隔离。合法生命周期覆盖可返回指针。

设计执行更正：当前公开核心没有 pin/description/note 编辑命令，不能新增API来满足测试清单，也不把fixture改值称为命令。使用现有完成、撤销完成、manual progress、carryover、失败命令验证保留旧结果；全部指针保真由helper与公开图ownership验证。同日priority重复被现有validator拒绝，time/ID tie-break测试单列内部比较器specimen，并验证constructor拒绝，不冒充合法公开图。

## 环境与运行边界

Go `go1.27.3 linux/amd64`；`build-environment/activate.sh`；显式外置 `day-read-build`；复用 `subtask-integration-build/go-cache`，未删除cache。GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB沿用环境，未调GC求通过。所有新旧微基准相同环境和fixture、计量外构造验证，无wall-clock阈值。

性能与逐命令exit见 `VALIDATION.md`。未运行真实年度development路径、root/full、hosted、六目标或完整四模块聚合；这些需协调与后续同来源版本验证。原600s未改，正式hosted root记录592.886s，仅余7.114s名义裕度；微基准优化机制不证明该风险已解决。
