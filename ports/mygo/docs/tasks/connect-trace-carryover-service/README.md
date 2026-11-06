# 普通任务延期／延续持久化

Task-id：connect-trace-carryover-service。

隔离 development/e2e Service 提供 DeferCurrentTrace(chain, trace, target) 与 ContinueUnfinishedTrace(chain, trace, target)，只有 canonical audited aggregate 写入并回读确认后返回新 trace ID。基线为 ddee25c8fc6f8e7ae7ee1251277654f8095cbd12；固定源语义为 43e6eefaae3a04be89cab3e234d470111afd80ef。

- 延期将今天 pending source 结为 deferred，新增一个源空分类 terminal snapshot/event，并复制开放子任务、lineage、描述、有效备注及进度。
- 延续保留 unfinished source 全部事实和分类字节，新增连续序号及新 trace/child 身份。
- 只支持严格空 catalog 普通任务；其他 supplement 仍 fail-closed，不放宽 codec/import/domain validator。
- deferred 是开发 reader 的明确范围扩展。旧 reader 不能打开该新会话，回滚不得删除、降级或改写已落盘资料。
- 新 ID、内存图、snapshot 和成功确认都在 commit 后才发布。after-persist 故障无成功确认，句柄封锁，重新打开可恢复真实已落盘结果。
- 本批没有 UI 接线，不代表已完成／未完成页面、中文本地化、Swift oracle、生产存储或非 Linux 实机已验收。

设计见 DESIGN.md，验证见 VERIFY.md，直接通用审计绕行的实证修复见 ../../engineering/failure-cases/GO-RUNTIME-014.md。
