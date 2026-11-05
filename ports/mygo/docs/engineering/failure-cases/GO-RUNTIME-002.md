# GO-RUNTIME-002：已打开会话历史损坏后仍显示保存成功

状态：处理中，已补开发路径红→绿回归，真实桌面症状门禁未运行。

- 首次发现：2026-11-05，独立只读审查的合成复现。
- 引入提交：4cbd56883b8a6065c82902923b66ecc0741a2cef。Git 作者身份不等于实际操作者，不能仅凭 Git 归因。
- 症状：保持服务打开，修改旧 state.json 末字节或删除 SESSION_IDENTITY，后续新增仍返回成功，但重开失败。扩展复现确认较早历史、较早 COMMITTED、身份变更和整个 session 目录移动也有同类问题。
- 错误机制：commit 只检查旧文件类型/长度和最后一个 COMMITTED 字符串，未验证完整字节 hash 链与当前目录身份。
- 修复：共同 verifyPrefix 边界在 reservation 前及新 revision 完成后各检查路径实际目录身份、marker、连续 revision、完整 state bytes/COMMITTED hash/父 hash 链与 handle 预期 head。写前失败不 reserve、不发布；写后仅核对自身已提交前缀，允许合法后继 writer，以免将已成功的提交误报失败。写入后的不确定结果统一标为 ErrIncomplete 并冻结 handle。
- 快速门禁：TestOpenHandleRejectsHistoricalCorruptionBeforeReservation、TestLateHistoricalCorruptionPreventsPublication、TestLaterWriterBeforeAcknowledgmentDoesNotCauseFalseFailure。原先七种故障均为红灯，修复后全部转绿。
- UI 症状门禁：TestLiveUICorruptHistoryRefusesSave，经真实 MyGo 控件输入/点击，确认不显示成功、保留草稿和旧快照、没有新增 revision。进入 scripts/check；未冒称系统窗口验证。
- 风险边界：只能检测检查时可观察的完整性，不是同账号恶意篡改隔离或签名。原历史不自动修复、覆盖或删除。原三批模块未修改，生产资料未接触。
- 回滚：不要恢复到缺少完整性门禁的版本；不完整开发 session 保留检查，另建全新隔离会话，不自动迁移。
