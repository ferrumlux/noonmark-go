# 标题草稿会话接入

日期：2026-11-06（新加坡）。本批仅接入无 IO、调用方串行使用的 internal/titlesession；界面仍只读，原生文本最终化、实际 timer 和关闭协调另行验证。

设计：复用正式 inputdraft Gate，不复制 Gate、不引入第二套 debounce 状态机。会话拥有原始 draft、native 快照、最后实际落盘字节、不可修改 identity/job/request，输入变更使旧最终化证据失效。普通候选 700ms、flush 0ms；成功只能由同一 Service 事务的真实确认字节驱动。所有 IO 及原生调用由外层持有，不允许多个 goroutine 调用同一 session。

LifecycleFlushClean 仅证明调用方送入的快照，绝非直接 Close 许可；每次真正关闭回调仍须主线程最新 native Finish 后再验证。错误保留草稿并锁存本次关闭失败，Conflict/Incomplete/Session 等永久会话错误须由外层停止重试。空白普通输入不写、不丢；生命周期空白恢复最后确认文字，等待已有保存完成，不恢复旧 ack。

保留的普通 blur 边界：保存 A 在途时输入空白并 RequestFlush，A 成功会清 canonicalizes，随后 0ms blank job 只消费作业，保留 blank+dirty。这与上游 AutosavingMarkdownEditor.swift:221–235、302–327、355–375 的普通 finishEditing/persistDraft 一致；再次明确 RequestLifecycleFlush 才恢复最新 A。导航/关闭不得只依靠普通 RequestFlush。独立反例已固化，不暗改为相似但不同的行为。

本次复制候选的 session 实现与原29测试，调整正式 import，并添加独立普通 blur 反例、真实 Service 合成资料保存/重开与拒绝时保留草稿测试。完整候选设计细节保留 SESSION-REVIEW.md。未经 Swift 动态 oracle 或真实 OS IME 验证。

最终集成验证：33 项顶层 session 测试（含独立 blur 边界及真实 Service 合成写入/重开/拒绝测试）通过；聚合 scripts/check race/vet/gofmt 全过，主包35.375s、session1.342s，其他未改包使用测试缓存。六目标编译和 CLI development 新建/重开/拒绝复用通过。外置证据为 evidence/title-session-{check,cross,entrypoints}-final.log。
