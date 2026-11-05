## 当前状态（2026-11-06，新加坡）

日待办 pending 且 date>=today 的原生多行标题编辑已接入隔离 development 应用：700ms自动保存、真实Handle快照／最终化、统一写入lane、可取消Close／Quit及失败留稿。完整实现、最终证据与安全差异见 [NATIVE-UI.md](NATIVE-UI.md)。下面报告保留为当时的前置设计／验证记录，其中“只读／尚未开放”等描述只适用于该历史批次，不代表当前运行状态。

Linux headless与六目标编译不是实机IME或Swift动态oracle；这些限制及原始未修复依赖红色探针继续保留。

# 标题应用协调器

日期：2026-11-06（新加坡）。本批先接可测试的 internal/titlecoordinator，不改变实际只读UI；MyGo/Service真实接线另行验证。

设计：单一FIFO保存lane只读不可变trace/text/request，实际执行时采样自然日，AdvanceTo成功后才调用同事务字节ack。UI串行阶段拥有session/native/timer，worker只通过异步Post交还结果；UI不等待worker。相同job保留deadline、0ms立即在安全阶段pump。普通blur保留源RequestFlush；导航先Lifecycle确认再变更视图，这是与Swift先导航后retiring保存的明确安全差异。

每次真正BeforeClose/BeforeQuit必须新native最终化；不会用旧clean布尔或timeout放行。注册表epoch和currentIntent防同步hook新增/移除editor漏检；变动最多异步重新验证一次，持续变动明确失败，不无限spin。ReleaseClosed若发现新dirty恢复close意图并推进保存，completion只能重入真实Close。clean历史trace禁止再写但允许fresh确认后关闭/转readonly；dirty/inflight/失败草稿仍保留。

SynchronizePersistedText接已核实有序publication，先读真实native，dirty/IME/inflight不被覆盖。每entry publication token防nested B->hook C->outer B将新值退回；旧调用安全no-op。来源没有版本/顺序证明的延迟Snapshot不可冒充已核实更新。Native.Replace必须由真实适配器再次核验最新native输入，不能只写应用draft假称成功。

五项独立反例原样保留（只改package/import/gofmt）：quit漏新注册、release漏新注册、clean历史永久拒关、release新dirty无进度、nested有序publication倒退。最终候选48项测试race/vet通过，覆盖率92.0%，独立复审原5反例通过。核心实现改包名前hash4f620d0477616296c6e5845d23de2a88369e96a980572eba4a00c9abee868ef9。

必须由真实应用补足：pending注册关闭窗口、native帧间未绑定输入与Replace证明、统一普通命令保存lane及在途退出保护、自然日后旧draft可见、跨窗发布顺序、真实Close/Quit、持久化故障及重开。现有mock通过不等于上述已接通，更不等于三平台实机/IME/Swift oracle。

正式集成验证：scripts/check通过，应用主包race34.022s、titlecoordinator48测试race1.015s，其余未改包使用测试缓存；全模块vet/gofmt、50项依赖门禁、37项prepared-native回归与vet通过。六目标编译、development CLI新建/重开/拒绝复用通过。外置证据为evidence/title-coordinator-{check,cross,entrypoints}-final.log。此批没有启用编辑UI，不以mock替代下一批实际接线。

## 真实 UI 接线前的在途回归修正

真实输入与重复关闭的接线测试发现：旧标题保存尚在途时，新输入可能产生已到期的候选。协调器若此刻再次 BeginPersistence，会把正常的单一在途门禁拒绝误当失败，并清除待完成的关闭意图。现 pump 保留该候选，等待当前请求完成后再按最新 native 快照推进；不跳过输入读取，也不允许并行写入。

新增独立回归覆盖 close、quit、navigation、普通 blur 及 observe 五种重复路径，断言 A 与 AB 各自按序提交，旧确认不能提前关闭，最新意图最终继续。修改前 observe 路径实际复现三次 ErrNotReady；修改后全部协调器测试及 race/vet 通过。证据为 evidence/title-inflight-review-red.log 和 title-inflight-{check,cross,entrypoints}-final.log。此修正仍不单独宣称实际 UI 或原生平台验收完成。
