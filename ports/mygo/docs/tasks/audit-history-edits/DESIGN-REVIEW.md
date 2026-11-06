> 阶段归档：保留原设计或隔离副本验证边界，正式整合状态见 README。

# Title / manual progress 封闭历史设计独审

## 结论

**Accepted，仅批准设计进入既定范围的实施与独立验证。没有阻断项，不代表实现通过、Service/UI 可用、Swift oracle 或跨平台已验证。**

本次只读核对真实 Go 与 Swift 源码，没有运行测试、编译、构建、Git、网络、缓存或生产资料操作；仅在本独审目录写报告。未修改作者设计或正式源码。提交身份采用委派给定的 Go `2a808a3c4fe788c69d8f47cf21e1d686860a19ff` 与 upstream `43e6eefaae3a04be89cab3e234d470111afd80ef`，没有通过 Git 自行复核 HEAD/clean。输入文件 SHA 是本次实际计算，不只照抄设计。

## 精确证据

- DESIGN.md：`760183532ea0283ff9b0694cc6e84a758e3335a7cc8c1bdf8488d1a90a1fd299`
- TEST-MATRIX.md：`7d260fb71b7ed1cf7df8f2d877380c33522c7501690a5ae96dad6c4a6d0e4838`
- SOURCE-SHA256.tsv：`062c14760bbfce1b24b594298b4b96cd66255a434bb629e4e8c8044c2570745b`
- manifest 41 项实际文件全部匹配。完整逐文件核验及额外 bridge/schema 源码哈希见 VERIFIED-SHA256.tsv。
- 已读 Go ports/mygo/AGENTS.md 与上游 AGENTS.md，按本轮隔离副本与只读审查授权执行，不采用上游针对另一 Linux 主机的历史固定路径。

主要人工核对：core/title.go、manual_progress.go、progress.go、time.go，models/validation 相关身份与定义约束；taskapp/history_command_vnext.go、history_command_vnext_budget.go、其 AST order gate、history_lifecycle_command_vnext.go、history_vnext.go、history_vnext_builder.go、taskapp/title.go；v7/domain.go 与 schema；Swift NoonmarkEngine.swift 1216–1320、2842–2864、3556–3568、3937–4019。其余 manifest 文件的 hash 一致不等于逐行人工审阅。

## 逐项判断

1. **预算先行成立。** 四条 gate 为 seal、before、after、input，然后立即 bind；现 bind 会 clone、ToDomain、FromDomain/Validate，所以输入 gate 放在这里之前必要且可实施。新增 input 仅 string header/固定标量，累计 Title、ChainID、TraceID、NewDefinitionID 的逻辑使用；alias 仍逐字段计费。两图和输入三份独立限制，不合并 16MiB，也不以最坏 6x JSON 转义误拒。现 graph budget 必须原样复用，真实 envelope 限制留给后层。
2. **标题未收窄已有合法分支。** Go 与 Swift 都先做状态/历史 pending/锁日 guard，再标题相等 no-op；SaveInput 的 reuse 取决于 current 是否被非 pending history 引用，而非是否有任何 history。无历史 Rename 亦 reuse；有 R 的 chain route 可编辑，不要求额外 P。版本化只开放旧 current 三项 supersession metadata，历史正文冻结正确。现 core 明确允许 restored current 留 successor witness，设计没有误拒它。
3. **未消费参数保持未消费。** core/title.go 在 equivalent 返回之后才验证 Now，并只在 version 分支验证 NewDefinitionID。设计明确 no-op/reuse 不验 unused ID 的 UUID、长度36、冲突或 UTF-8，也不预留；资源总量仍受限。manual 同样在 next 与已有非 nil pointer 相等后才读 Now。NaN/Inf 的请求身份可以保留为 Instant bits，不进入 snapshot。canonical-equivalent title 必须保留原字节而非返还输入归一化后的字节。
4. **route 与双端绑定充足。** trace title 只允许 SaveInput，选中 trace 明确绑定 ChainID，再检查选中 trace 日期与 P，而非找同链另一 P 替代。chain route TraceID 空形状封闭。历史输入从 prior owned graph replay；caller before 与 sealed prior、caller after 与整个 replay 都通过现 bind/match 验证，不存在只校目标漏掉其它字段的设计通道。request action/route/title/newID/percent 与已有 chain/trace/today/now 均保留私有 scalar 身份；无未绑定可变 request 对象。
5. **零 capture 并非无审计。** finish(nil) 仍执行 builder、classification 深复制、FromDomain、full inspector、prefix、supplement fence、exact count。新独立 fences 不替代 replay，不提供 caller exemption。分类 tamper 负例放 builder/prefix 层是正确分层，不能假称不存在的 audit classification 参数可注入。
6. **独立 fence 可实施。** title 先逐位置/ID 守住数组数量与旧顺序，版本分支恰追加一份定义；非目标链、非 pending traces、所有其它 definitions 都冻结。manual 只有目标 trace 的两字段可变，顶层 key 集及所有其余值固定。projection 必须建新 object，不能在 prior map 删除字段。用 owned/replay 定位 current 与新增定义即可，不需再写一遍领域分支算法。
7. **manual 规则符合原核。** Go progress projection 与 Swift hasSubtasks 都覆盖全链所有 formsDayHistory traces 的 presentable children，含未来日，不是只看当前目标；cancelledDraft child 不阻断，definition 规划子项不是 materialized child。floor 是更早日或同日较小 continuationSeq 的历史 raw 最大值，同组 peer 不入 floor；nil→0 是变化，100 不完成且不写 chain clock。M01–M07 已覆盖主要值域、顺序与历史延续；应把未来链内 child 和仅 planned children 明确展开成子例。
8. **结果边界合理。** 同一 package-private closed result 增三 kind，只由两个新 audit 的成功出口构造，private seal 与 owned snapshots 不外泄。沿用 snapshotCopy 深复制；字符串值与 Instant 原始 bits 可保存，不保存 caller 指针、切片或 graph。valid 不是签名或提交权，包内代码仍属受信实现边界，AST 限定 constructor 与无公开 factory 必须落实。失败每出口返回零结果。
9. **可达性分类没有越权。** [A]P/[A,A]P、[A,U]/[U,A,U] 必须逐步实际核心命令及已有 typed audit 组合，每步 inspector/prefix；U 无 P/R 不能改标题，manual 要另一合法当天 P 或明确 core-only continuation fixture。negative epoch、±0、MaxFloat、restored witness、边界容量是 inspector-valid 参数/profile 证据，不可宣称当前 Service 生成。未完成全部 consumer 对等仍不得 cutover；不要求本批一次补全未来 writer。

## 实施时必须保持的精确解释

以下是现设计可接受解释与验收细化，不是准许扩大范围；建议作者后续同步文字，但无需为了它们改正式旧代码。

- `historyCommandInputFields` 若采用，在新 `history_edit_command_vnext_budget.go` 显式定义不冲突的 typed constant；旧 budget 文件连 enum 都不改。设计 §8 已把旧生产修改限于 history_command_vnext.go，不能把 §3 的“可增加 phase”误读成第二个旧文件编辑授权。
- title chain clock 的 `max(old,Now)` 是 core 的严格 `Now.After(old)` 条件，不是对 uint64 Instant 做整数 max。数值相等（尤其 +0/-0）应保留旧 chain bits；no-op 根本不算 max，不验证 NaN/Inf。旧 event、未改字段保持 exact bits；允许变化字段写入 core 实际给出的 Now bits。请显式加入 old=-0/Now=+0 以及反向、全负时钟、equal 修改和 no-op NaN 的独立预期。不能加全局 >0 floor 或严格后继。
- plannedSubtasks 在 wire schema 是必有数组；v7 encodeTyped 把 nil 与空 slice 都写成 []，decodeTyped 用 MakeSlice。因此“保持 nil/presence”不能虚构 wire 上 nil/empty 的区别，或拿 caller before 的 nil 去拒绝 owned decode 的空 slice。optional description 的缺席与空字符串确有 wire 区别，必须保存；在 core 层保留 nil 的实际 clone 分支可另外验证。TEST-MATRIX F02 已给出正确限定。
- manual owner mismatch 的 adapter typed owner fence 与核心 ErrInvalidInput 不应混写成同一断言：若按 §4 采用显式 owner guard，就固定其 typed owner 结果；若仅调用原 core，则其领域错误原样。这是错误层级表述细化，不能改变 wrong-owner 必拒与预算先行。title trace owner 则已明确是 typed owner。
- 所有非法 action/route/必消费 ID 内容检查均在预算后。metadata helper 不宜靠提前返回某个“无使用”分支跳过其它字符串计费。没有 optional NewDefinitionID validator，即使字段有任意 UTF-8 无效字节也只在真实 version 交给 core 消费。
- 新 AST 检查不能仅搜字符串或仅检查第一个调用；维持旧 gate 完整初始调用序列与立即 bind 断言，并解析新文件、新入口、新 helper；新增 proof 构造限制只针对两个新 audit，原四入口继续合法。逐函数 body 哈希需在实施前取基线，不能实施后重建基线掩盖改动。

## 最小实施 allowlist

仅 `ports/mygo/internal/taskapp/`：

1. 新 `history_edit_command_vnext.go`：两强类型 audit、固定 route/shape 及独立 fences、三种精确 core 调用。无 Service/store/allocator/clock/frontier/reader 接线。
2. 新 `history_edit_command_vnext_budget.go`：两 metadata-only input budgets 和可选 typed input phase constant；复用现算术，不修改旧 budget。
3. 新专用测试文件，含独立 expected/fence、alias、旧字节、时钟 bits、fixture 谱系与 AST/source freeze 门禁。
4. 旧 `history_command_vnext.go` 仅追加三个 kind、必要 private scalar metadata、四→六入口注释，以及 valid switch 三 kind 白名单。其它现有函数身体逐字节冻结。
5. 既有测试文件仅增新解析文件/入口/helper 覆盖与 closed-result 新 kind 断言，保留旧断言及未知 kind 拒绝。不得删除、弱化、skip 或扩大通用 allowlist 来迎合新实现。

没有授权修改 core、r1、Service/store/UI/v2、现 bind/match/finish、旧四 audit 或任何旧生产函数身体（valid 指定例外除外）。没有授权修旧预算算法、引入 writer 或新 allocator。超出此表必须返回设计审查，不直接扩展。

## 后续验收边界

实现者先取得函数 body 冻结基线，再做新单元及独立 review，随后跑原全部 fast gates，不削减。当前报告所有行为判断为静态源码及结构推理；内存/alias、时钟、资源、历史一致性需要真正运行实现测试才能形成验证结论。Swift Foundation 空白等价、Apple oracle、真实 UI IME/owner 切换/退出保存、各平台运行均未在本次执行。此次 Accepted 不能对外替代这些证据。
