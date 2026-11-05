# 标题编辑：专用历史审计与隔离事务

2026-11-05。范围：正式 `ports/mygo` 的 isolated development/e2e 聚合；没有接通 UI、生产身份、真实 AI、分类 catalog 或周期任务。核心语义研究见 RESEARCH.md。本阶段只新增 `internal/taskapp/title.go`、`title_test.go` 与本文；没有放宽既有 `CaptureEmptyHistory`，没有更改既有 service/store 文件。

## 两个服务入口

- `Service.RenameTaskTitle(chain, title)`：显式重命名，按核心规则在已有日事实时创建定义后继。
- `Service.SaveTaskTitleInput(chain, title)`：输入保存；没有历史引用的当前定义可复用，首次隔离历史后，后续输入不会继续增加定义。

两个入口共用一个服务锁。锁内读取 today、计算包含全 graph frontier 的 nextInstant、生成候选身份、执行独立 kernel 命令、审计完整 snapshot、走原有 normalizeSnapshot 的 encode/decode/ToDomain，最后只做一次 disk.commit。只有 commit 完整成功，才同时发布 graph 与 snapshot。标题事务不附带日结算，不产生分类事件，也不更新 today。

空标题可由核心和这两个服务入口持久化，不代表详情 UI 应保存空白。尚未开放 UI 编辑；IME、700ms debounce、空白失焦恢复、退出 flush 与 trace 级失效 callback 路由仍待下一阶段。

## 命令专用证据，不是通用豁免

私有 `titleCommand` 只能包含 rename/input 枚举与 `core.EditTaskTitleInput`；未知枚举拒绝。没有通用 allowHistoricalMetadata 开关，也没有 caller 提供的字段忽略名单。

`captureTitleHistory` 的顺序：

1. 对 prior 执行严格 ValidateEmptyHistory，拒绝缺失/伪造结算事件；独立 Inspect 副本避免修改调用者输入。
2. 使用 prior 完整 supplement 编码 before，要求整个 snapshot 与 prior canonical 字节一致。
3. 从 before 创建独立 kernel，重放具体标题命令，要求候选 after 的整个 canonical snapshot 与重放结果完全一致。额外 trace、定义、子任务，或偷偷修改任何字段均不能借用标题权限。
4. 独立防线冻结所有 day、所有 materialised subtask、完整 classifications，以及所有非 pending trace 的 canonical 字节。历史定义内容逐字节冻结，只容许尚 current 的历史定义合法向前 supersession。
5. 这一次 supersession 必须指向本次新建、同链、maxSequence+1、createdAt/contentUpdatedAt=Now、未再 superseded 的后继。旧定义只可更新精确的 contentUpdatedAt、supersededAt、supersededByDefinitionID；后继 description/plannedSubtasks 必须完整复制。旧 current 可以带合法 restoration witness，不要求旧 pointer 为空。已 superseded 的版本不能借机重写。
6. 最终再次 ValidateEmptyHistory。服务随后仍须完成 normalizeSnapshot；专用 history 审计没有取代完整 runtime scope 验证。

完整 extra 沿用 prior；分类 revision、events、latest snapshots 不变。审计不重建缺失历史。未锁日 completed 对当前定义的历史引用也受保护，不仅锁日才算历史。

## No-op、冲突与不确定写入

normalized 相同标题以及 Swift String canonical-equivalent 字符串比较形成的核心 no-op，保留原有储存字节和所有时钟，不新增磁盘 revision。两个入口仍执行权限/日期验证、完整审计、normalize，并检查 disk prefix；stale handle 即使提交本地相同标题也返回冲突。blocked handle 在开始即拒绝，不能在 after-persist 不确定状态下以 no-op 冒充保存成功。

四个故障阶段都不发布内存候选：

- before-reserve：没有新磁盘 revision，重开仍是旧值。
- after-reserve / before-commit：保留不完整 revision，重开 fail-closed，不能自动清理或重试为成功。
- after-persist：可能已有完整持久化 revision，但原 handle 保持旧内存且冻结；重新打开验证后可读取新值。这是明确的不确定结果，不声称事务完全回滚。

跨句柄以现有磁盘 prefix 和 exclusive revision reservation 防竞争；同句柄由服务锁串行。自然日推进与标题保存也在同一锁下决定先后，结算后拒绝修改历史。

## 自动化验证

以全合成资料覆盖：

- pool 与今日 pending 的原位 input；显式 rename 分叉；空标题、多行规范化、重开恢复、相同输入 no-op。
- 已锁 completed 旧定义与同链当前 pending 的合法合成图：首次 input 分叉，二次复用，历史标题、详情、planned/materialised subtasks、trace 与 audit 完整冻结；保存后重开仍相同。该 fixture 为未来生命周期准备，不表示现 UI 已能创建这种同链历史。
- restoration witness 与非连续 maxSequence、nil optional description、已 superseded 版本冻结。
- 恶意修改历史 title、description、plannedSubtasks、createdAt、sequence、trace、subtask、day review/lock；新增/删除历史事实；后继跨链/错误 sequence/clock/内容、错误 pointer、额外 definition；错 mode/clock/ID/title/prior/before、缺失或被篡改 audit。拒绝后输入不动。
- 结构合法的非空 category、recurrence、returnedToPool、cancelledDraft 仍被 runtime 拒绝；非 neutral preferences 在 normalize 阶段拒绝。
- 四个磁盘 fault 阶段与重开核对；stale、blocked、closed；并发输入、跨句柄竞争、自然日竞争；完整 frontier clock；audit/normalize 失败不触及 reservation。
- 两入口预组合 café 与分解 cafe+U+0301 的 canonical 等价 no-op 不改任何字节或 revision，stale/blocked 时仍拒绝。

验证命令：`go test -p=1 -race ./internal/taskapp`、`go vet -p=1 ./internal/taskapp`，使用工作区 Go 与指定外置 cache。Linux 合成单测及 race/vet 通过；没有运行 Swift oracle、Apple UI、真实 IME 或非 Linux 实机。不能据此宣称全平台或生产持久性已验收。
