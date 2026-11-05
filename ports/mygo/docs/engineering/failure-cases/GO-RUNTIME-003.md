# GO-RUNTIME-003：未分类任务结算遗漏空分类历史

状态：开发回归及独立复查通过；修复与本案例同一提交，真实桌面门禁待补。

- 首次发现：2026-11-05，对照上游源码并经独立复现。
- 引入提交：4cbd56883b8a6065c82902923b66ecc0741a2cef。Git 作者身份不等于实际操作者。
- 症状：跨日后普通任务已转 unfinished/completed 且日期锁定，但分类 revision 仍为 0，snapshotEventsByTraceID 为空。
- 上游依据：NoonmarkEngine.swift 的 settleDays 对新 unfinished 与 completed 调用 TaskClassification.swift 的 captureClassificationSnapshot。即使 category=nil、labels=[] 也创建事件及 latest snapshot，并递增 revision。
- 错误机制：运行服务只持有 TaskGraph，encodeGraph 每次重建 neutralSupplement；“没有分类就没有历史”的论证不成立。
- 修复：服务持有任务图与完整候选 snapshot 聚合。在同一 candidate 中结算及 CaptureEmptyHistory，严格 audit、编码、单次存储成功后成对发布。load/save 无条件检查 empty-history 生命周期，不依赖通用 codec 的 revision-zero shortcut。
- 范围：只接受空 catalog 的历史子集。拒绝真实分类/标签/关系/merge/receipt 等其他非空状态，不放宽通用 v7 audit 门禁，不声称 Swift oracle 已验证。
- 回归：TestAggregateRolloverCapturesAndPreservesEmptyHistory、TestAggregateRolloverFailurePublishesNeitherPart、TestAggregateRejectsZeroedAuditDespiteCodecShortcut；UI 的 TestLiveUINaturalDayAdvancesBeforeCommands、TestLiveUIBoundaryFailureBlocksCommandsAndPreservesDraft。
- 开发资料 clean cut：新身份及私有记录为 v2，v1 明确拒绝，不自动修复、迁移、删除或覆盖。旧目录完整保留，应另建全新 development session。
- 自然日：打开、每分钟轮询及命令前推进；手动选择的历史日期保留，跟随今天的窗口一起前进。操作系统时区/唤醒通知的原生全量对等仍未验证。
