# 普通分类 Service 精确源码门禁

## 范围

本批整合普通分类完整状态、保真 codec、私有准入与 Service 持久化组合，只使用显式 `CreateOrdinaryDevelopment` / `OpenOrdinaryDevelopment` 进行隔离合成资料验收。默认 `Create` / `Open`、默认 codec、空 catalog 历史门禁及 UI 入口没有改为普通分类模式。没有新增生产导入、管理、labels、AI、cycle 或跨进程自动重试能力。

现有冻结链在 canonical quote 阶段结束时包含 73 件生产源码。实测旧门禁首先拒绝 `carryover.go`；逐件比对发现精确七件已有生产文件变化，均来自普通分类 Service 接点。其余 66 件保持原 hash。新增严格第七阶段 `ordinaryServiceFrozenOverlay` 仅承认下列七件固定旧／新 SHA256 组合，不按目录放行，不以当前工作树自动刷新基线：

- `internal/taskapp/carryover.go`：延期／继续接入封闭 ordinary task transition。
- `internal/taskapp/manual_progress.go`：进度变更与真实 acknowledged outcome 接点。
- `internal/taskapp/return.go`：回池接入 ordinary transition，历史 capture 由封闭边推导。
- `internal/taskapp/service.go`：显式模式分流、日结、clock、Close 与现有任务命令接点。
- `internal/taskapp/store.go`：逐 revision ordinary admission、真实 Service/head 绑定及封闭候选 writer；保留原 reserve、提交标记、同步、readback、uncertain 协议。
- `internal/taskapp/subtask.go`：现有子任务命令与 ordinary acknowledgement 接点。
- `internal/taskapp/title.go`：标题命令与 ordinary transition 接点。

这里只冻结所列源码身份，不从 hash 推导功能完整性。完整 Service 路径的判断仍依赖对应行为测试。

## 固定清单与强约束

- `FROZEN-BEFORE-v1.tsv`：旧六阶段完成后的完整 73 件清单。
- `FROZEN-PRODUCTION-OVERLAY-v1.tsv`：精确七行 `旧 SHA256 → 新 SHA256 → 路径`。
- `FROZEN-AFTER-v1.tsv`：新增第七阶段后的完整 73 件清单。

旧历史 manifest 与六个 overlay 文件没有改写。旧 consumer 的生产 hash 循环、旧 audit 后缀 hash 断言和各阶段拒绝测试全部保留。`day_read_freeze_test.go` 中唯一原断言扩展，是在原六项有序 `want` 列表末尾明确追加第七项；没有改筛选条件、缩减原列表或允许任意阶段。

曾考虑另用不匹配原 AST 枚举的接点名称，该方案已撤回，在该版本上没有运行门禁或发布。实际实现使用明确的 `FrozenOverlay` 名称，并让旧 AST 断言识别新增阶段。

新阶段严格核对完整输入集合的 73 件路径及 hash、固定 manifest bytes，并在全部检查成功后复制 map。缺行、重复、额外路径／列、alias、错误 old/new、空输入、格式变化、任意基线缺项／前进／改写均拒绝，失败不修改输入。新测试同时核对公开 before/after 清单、原六阶段后追加顺序、结果赋回 `expected` 且早于原生产 hash 循环。结果 map 不与输入共享。

## 已执行的有限检查

- 逐 phase 核对 R4、aggregate、codec 三次提交、Service 三次提交的原始 diff 与整合 diff，八段完全相同；没有人工改写产品组合。
- source 冻结链、overlay 正负例及专属 consumer scope 检查使用 race detector。
- ordinary admission、Service 全命令四象限矩阵、原 plan／retry、capture、writer 故障与绑定测试，以及默认空 catalog 边界测试使用 race detector。
- `internal/taskapp` 的 `go vet`。

以上只证明本次有限检查范围。全四 module／完整 `scripts/check`、cross-build、entrypoints、Swift 完整状态对照、macOS／Windows native、真实 UI／IME、断电耐久与长期资源峰值均不在本次已执行范围。默认入口未启用普通分类，不能把显式 development 接口测试表述成完整产品或源兼容验收。
