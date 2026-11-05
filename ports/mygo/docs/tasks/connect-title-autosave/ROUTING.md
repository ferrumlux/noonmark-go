# 详情标题的原子 trace 路由

2026-11-05。`Service.SaveTraceTitleInput(traceID, title) error` 是 Day 详情输入的 adapter，保留已有链级 API。

旧链级输入允许“旧历史 trace + 同链当前 pending”的合法链继续编辑，但不能据此让旧详情的失效回调写入当前任务。新入口同一把 Service 锁内检查 closed/blocked、查所选 trace、拒绝 date<today 与非 pending，再调用私有 editTitleLocked 执行原有核心命令、专用重放审计、规范化、持久化及成功发布。没有先 Snapshot 校验后重新加锁的 TOCTOU 窗口；没有以当前定义 ID 替换 trace 路由，也没有放宽旧历史冻结。

canonical-equivalent no-op 也必须先通过 trace 范围检查，stale/blocked handle 保持拒绝。自然日与标题操作竞争在同一锁内串行：先保存则结算冻结新值，先结算则旧 callback 被拒绝；重开结果与接受顺序相同。界面仍需在发起任务前协调自然日刷新和草稿生命周期，本 API 不自行改 today。

全合成回归包括今日/未来、多行规范化及重开、同链历史回调攻击、missing/completed/past、canonical no-op、stale/blocked/closed、四种写入故障及重开、八轮自然日并发。初次历史测试把未规范化的 nil slice 手工装入 Service，产生 nil/[] 的比较差异；定位后改为与实际入口相同的 normalizeSnapshot 初始化，没有修改产品实现或弱化内容冻结断言。

Focused `go test -p=1 -race ./internal/taskapp -run TestTraceTitle -count=1` 通过（2.832s），taskapp vet 通过。最终聚合和平台边界另见本批 VERIFY.md；不代表真实 UI、IME 或 Swift oracle。
