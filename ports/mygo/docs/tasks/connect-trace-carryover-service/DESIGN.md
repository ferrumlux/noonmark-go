# 普通任务延期／延续：持久化设计

## 需求与边界

在独立副本（基线 ddee25c8fc6f8e7ae7ee1251277654f8095cbd12）接通显式 chain、source trace、target date 的 Service 命令。保留 AGPL；不改 UI、正式仓库、reference、MyGo、production、iCloud、Keychain、AI 或远端。只接受经过严格审计的空 catalog 普通任务，重复任务及其他 supplement 继续拒绝。HistoryProjection 是纯投影，不代表已完成／未完成页面已经接通。

## 已核对的来源

上游固定 43e6eefaae3a04be89cab3e234d470111afd80ef：

- Sources/NoonmarkCore/NoonmarkEngine.swift:2152–2270：延期修改今天 pending source、清 pin、设 settledAt、新增 carried trace、复制开放子任务；延续保留 unfinished source、新增 continuationSeq+1 的目标。两个命令均无成功 no-op。
- 同文件:4124–4154：延期将开放旧子任务设 deferred；延续不改旧子任务。新子任务保留 lineage，以新身份／连续 position／source link 创建。
- Sources/NoonmarkCore/TaskClassification.swift:4022–4063：延期只为 source 新增一个空 snapshot/event，revision+1，captureAt=now；延续不新增分类事件。
- App/NoonmarkMacApp/NoonmarkStore+TaskMutations.swift:869–916：候选提交成功后才选择目标／显示成功；延续从 unfinished 页面发起时保留页面。此批不实现 UI 或 undo。
- App/NoonmarkMacApp/NoonmarkStore+NaturalDayPersistenceImport.swift:530–586：候选 snapshot 与 changedFrom 一起持久化。此端保持原开发存储整图 revision 事务，不宣称 SQLite 或 Swift 存储互通。

现有 Service 采用完整图克隆、canonical v7 规范化、严格空分类审计、append-only revision 和磁盘回读确认。现有审计拒绝 deferred，因此必须作窄域扩展，不能借泛用字段豁免。

## 实施顺序与接口

先等待 core 作者冻结并报告可复核文件／hash，核对后才导入本副本，不合入可变文件。预定 core 接口 TraceCarryoverInput 包含 ChainID、TraceID、NewTraceID、DayID、SubtaskIDs、TargetDate、Today、Now；DeferCurrentTrace 与 ContinueUnfinishedTrace 返回 error。

Service 同锁验证 closed／blocked，创建私有 kernel，分配新 trace/day/open-child 身份和全图严格单调时刻，执行指定命令。使用独立 replay 审计：prior 必须与 before 全图一致，重放同一完整输入，canonical after 必须完全一致。延期只追加 source 的唯一终端空事件，延续保留分类字节。normalizeSnapshot 后 disk.commit 确认，最后发布 graph/snapshot 并返回新 trace ID；错误返回空 ID，不更新 Today，不提前成功确认。

ValidateEmptyHistory 仅加入 deferred 终端类型：settledAt=contentUpdatedAt，日锁不得早于结算；唯一事件状态和时刻匹配 source，并保持所有现有 revision、身份、scope 约束。泛用审计不得创建或修改 deferred 终端事实，继续对既有终端 traces/definitions/subtasks 完整冻结。已有其他专用命令仍按完整 replay 审核。不补造缺失历史，不放宽 codec／import／validator。

## 验证与风险

实际 Service 命令合成所有 fixture，不手工制造业务历史。覆盖重复延期、跨日结算、延续链、开放与完成子任务、lineage/progress、历史 audit 不变、目标已有／缺失 day、错误 owner、重复调用、取消、目标日期及 guard 顺序。验证 exact replay 拒绝任意其他 graph 差异、缺失／伪造 audit 和非空 supplement。

四个磁盘故障点分别验证 graph/audit/Today/返回 ID 的失败原子性；after-persist 必须拒绝旧句柄成功确认，但重开可见真实已提交结果。覆盖 stale handle、并发 winner、prefix 损坏、极端 clock；先冻结文件再运行最终集中与全量门禁，并分别记证据。若发现源语义无法在现有受审格式安全表达，停止对应部分，记录具体 unsupported 字段。

风险：旧 reader 不接受 deferred，新 reader 的开发会话范围扩大；回滚只回滚源码，不删除、改写或降级 session。没有生产 cutover 或灰度，本批只能隔离 development/e2e。监测通过 typed error 和测试断言，不记录真实用户内容。Swift oracle、非 Linux 原生与断电耐久性未验证。

## 实施收敛与既有通用入口修复

已导入独立审查接受的 frozen core：carryover.go SHA256 8e18447e5db674a4a0922ef14834c0c8a5b576f0d32424310c77dbee3ded7dd1；carryover_test.go SHA256 c1bef356a8b5f042cbfe182de20c810b16b7f0a74d1f53a1dcc4076757d03df6。Service 对外两个方法均接受 chain、source trace、target date，返回 (DayTraceID, error)。只有规范化及磁盘确认完成才返回新 ID，所有错误返回空 ID。

实测发现 GO-RUNTIME-014：剥掉真实 continuation 的 trace carry link、sequence 及 child carry links，结构 validator 仍接受，旧 CaptureEmptyHistory 误当作普通新增 trace。这只证明直接通用审计入口的缺口，不证明正常 Service/UI 可利用，更不是生产事故。

最终根本约束见 GENERIC-BATCH-DESIGN.md：generic 新增 trace 时，使用一个私有 kernel 重放有界 Create／Schedule／Complete／Undo／Settle 命令见证，对完整 canonical after 相等检查。新旧 chain 一律真实消费池资格，不存在 eligibility-only fallback 或字段剥除。保留已证明的多链合法批次及完整历史验证；不声称恢复唯一真实历史或当时用户授权。专用 carryover 仍按调用方完整输入重放整图。
